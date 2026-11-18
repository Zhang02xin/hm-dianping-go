package shop

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Zhang02xin/hm-dianping-go/internal/localtime"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type shopReaderFunc func(context.Context, int64) (shopDetail, error)

func (f shopReaderFunc) FindByID(ctx context.Context, id int64) (shopDetail, error) {
	return f(ctx, id)
}

func testShopCache(t *testing.T) (*shopCache, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), ContextTimeoutEnabled: true})
	t.Cleanup(func() { _ = client.Close() })
	cache := newShopCache(client)
	t.Cleanup(cache.close)
	return cache, server
}

func TestShopCacheHitsMissingAndRedisFailure(t *testing.T) {
	cache, server := testShopCache(t)
	ctx := context.Background()
	var reads int
	reader := cachedShopDetails{cache: cache, source: shopReaderFunc(func(_ context.Context, id int64) (shopDetail, error) {
		reads++
		if id == 99 {
			return shopDetail{}, errShopNotFound
		}
		return shopDetail{ID: id, Name: "商铺", CreateTime: new(localtime.Time(time.Date(2026, 11, 18, 19, 0, 0, 0, time.Local)))}, nil
	})}
	for range 2 {
		got, err := reader.FindByID(ctx, 12)
		if err != nil || got.Name != "商铺" || got.CreateTime == nil || time.Time(*got.CreateTime).Hour() != 19 {
			t.Fatalf("shop read = %+v, %v", got, err)
		}
		if _, err := reader.FindByID(ctx, 99); !errors.Is(err, errShopNotFound) {
			t.Fatalf("missing shop error = %v", err)
		}
	}
	if reads != 2 || server.TTL(shopDetailCacheKey(99)) != shopNegativeTTL || server.TTL(shopDetailCacheKey(12)) != shopCacheTTL {
		t.Fatal("cache read count or TTL differs from expected")
	}
	server.FastForward(shopNegativeTTL)
	if _, err := reader.FindByID(ctx, 99); !errors.Is(err, errShopNotFound) || reads != 3 {
		t.Fatal("negative entry did not expire")
	}
	server.SetError("ERR simulated cache outage")
	if got, err := reader.FindByID(ctx, 12); err != nil || got.ID != 12 || reads != 4 {
		t.Fatal("Redis outage did not fall back to the reader")
	}
}

func TestShopCacheInvalidationRejectsLateRefill(t *testing.T) {
	cache, server := testShopCache(t)
	const id int64 = 12
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	reader := cachedShopDetails{cache: cache, source: shopReaderFunc(func(ctx context.Context, id int64) (shopDetail, error) {
		if calls.Add(1) == 1 {
			close(started)
			select {
			case <-release:
				return shopDetail{ID: id, Name: "旧值"}, nil
			case <-ctx.Done():
				return shopDetail{}, ctx.Err()
			}
		}
		return shopDetail{ID: id, Name: "新值"}, nil
	})}
	old := make(chan shopDetail, 1)
	go func() { got, _ := reader.FindByID(ctx, id); old <- got }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("first source read did not start")
	}
	cache.invalidate(ctx, id)
	close(release)
	select {
	case got := <-old:
		if got.Name != "旧值" {
			t.Fatal("test did not capture the in-flight old source result")
		}
	case <-ctx.Done():
		t.Fatal("in-flight read did not finish")
	}
	if server.Exists(shopDetailCacheKey(id)) {
		t.Fatal("old result repopulated an invalidated key")
	}
	for range 2 {
		if got, err := reader.FindByID(ctx, id); err != nil || got.Name != "新值" {
			t.Fatalf("post-update shop = %+v, %v", got, err)
		}
	}
	if calls.Load() != 2 {
		t.Fatal("new value was not cached")
	}
}

func TestShopCacheServesStaleWhileRefreshing(t *testing.T) {
	cache, _ := testShopCache(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	const id int64 = 12
	stale, err := json.Marshal(shopCacheValue[shopDetail]{Data: &shopDetail{ID: id, Name: "旧值"}, RefreshAt: time.Now().Add(-time.Minute).UnixMilli()})
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.redis.Set(ctx, shopDetailCacheKey(id), stale, shopCacheTTL).Err(); err != nil {
		t.Fatal(err)
	}
	refreshStarted, release := make(chan struct{}, 1), make(chan struct{})
	var calls atomic.Int32
	reader := cachedShopDetails{cache: cache, source: shopReaderFunc(func(ctx context.Context, id int64) (shopDetail, error) {
		calls.Add(1)
		select {
		case refreshStarted <- struct{}{}:
		default:
		}
		select {
		case <-release:
			return shopDetail{ID: id, Name: "新值"}, nil
		case <-ctx.Done():
			return shopDetail{}, ctx.Err()
		}
	})}
	if got, err := reader.FindByID(ctx, id); err != nil || got.Name != "旧值" {
		t.Fatalf("stale read = %+v, %v", got, err)
	}
	select {
	case <-refreshStarted:
	case <-ctx.Done():
		t.Fatal("background refresh never started")
	}
	for range 5 {
		if got, err := reader.FindByID(ctx, id); err != nil || got.Name != "旧值" {
			t.Fatalf("waiting readers did not receive the stale value: %+v, %v", got, err)
		}
	}
	close(release)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		got, err := reader.FindByID(ctx, id)
		if err == nil && got.Name == "新值" {
			break
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("background refresh never published the new value")
		}
	}
	if calls.Load() != 1 {
		t.Errorf("refresh called source %d times, want 1", calls.Load())
	}
}
