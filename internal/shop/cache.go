package shop

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	shopCacheRefresh = 30 * time.Minute
	shopCacheTTL     = 35 * time.Minute
	shopNegativeTTL  = 2 * time.Minute
	shopCacheIO      = 250 * time.Millisecond
)

var cacheStore = redis.NewScript(`
if (redis.call('GET', KEYS[2]) or '') ~= ARGV[1] then return 0 end
if ARGV[4] ~= '' and (redis.call('GET', KEYS[3]) or '') ~= ARGV[4] then return 0 end
redis.call('SET', KEYS[1], ARGV[2], 'PX', ARGV[3])
return 1
`)

var cacheInvalidate = redis.NewScript(`
redis.call('SET', KEYS[2], ARGV[1], 'PX', ARGV[2])
return redis.call('DEL', KEYS[1])
`)

var cacheRelease = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('DEL', KEYS[1]) end
return 0
`)

type shopCache struct {
	redis   *redis.Client
	ctx     context.Context
	cancel  context.CancelFunc
	slots   chan struct{}
	mu      sync.Mutex
	closed  bool
	workers sync.WaitGroup
}

func newShopCache(client *redis.Client) *shopCache {
	ctx, cancel := context.WithCancel(context.Background())
	return &shopCache{redis: client, ctx: ctx, cancel: cancel, slots: make(chan struct{}, 8)}
}

func (c *shopCache) close() {
	c.mu.Lock()
	c.closed = true
	c.cancel()
	c.mu.Unlock()
	c.workers.Wait()
}

func (c *shopCache) refresh(key, generation string, load func(context.Context, string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	select {
	case c.slots <- struct{}{}:
	default:
		return
	}
	c.workers.Go(func() {
		defer func() { <-c.slots }()
		ctx, cancel := context.WithTimeout(c.ctx, 4*time.Second)
		defer cancel()
		lock := key + ":lock"
		token := rand.Text()
		leaseCtx, releaseLeaseCtx := context.WithTimeout(ctx, shopCacheIO)
		acquired, err := c.redis.SetNX(leaseCtx, lock, token, 5*time.Second).Result()
		releaseLeaseCtx()
		if err != nil || !acquired {
			return
		}
		defer func() {
			releaseCtx, cancel := context.WithTimeout(c.ctx, shopCacheIO)
			defer cancel()
			_ = cacheRelease.Run(releaseCtx, c.redis, []string{lock}, token).Err()
		}()
		// A worker that queued behind another refresh must re-check freshness after acquiring the lease.
		checkCtx, cancelCheck := context.WithTimeout(ctx, shopCacheIO)
		values, err := c.redis.MGet(checkCtx, key, key+":generation").Result()
		cancelCheck()
		if err != nil || cacheString(values[1]) != generation {
			return
		}
		var header struct {
			RefreshAt int64 `json:"refreshAt"`
		}
		if body := cacheString(values[0]); body != "" && json.Unmarshal([]byte(body), &header) == nil && time.Now().UnixMilli() < header.RefreshAt {
			return
		}
		load(ctx, token)
	})
}

func shopDetailCacheKey(id int64) string {
	return "dianping:go:shop:" + strconv.FormatInt(id, 10)
}

func cacheString(v any) string {
	s, _ := v.(string)
	return s
}

type shopCacheValue[T any] struct {
	Data      *T    `json:"data"`
	RefreshAt int64 `json:"refreshAt"`
}

func readShopCache[T any](c *shopCache, ctx context.Context, key string, missing error, load func(context.Context) (T, error)) (T, error) {
	cacheCtx, cancel := context.WithTimeout(ctx, shopCacheIO)
	values, err := c.redis.MGet(cacheCtx, key, key+":generation").Result()
	cancel()
	if err != nil {
		return load(ctx)
	}
	generation := cacheString(values[1])
	reload := func(ctx context.Context, lease string) (T, error) {
		data, err := load(ctx)
		entry := shopCacheValue[T]{RefreshAt: time.Now().Add(shopCacheRefresh).UnixMilli()}
		ttl := shopCacheTTL
		switch {
		case err == nil:
			entry.Data = &data
		case missing != nil && errors.Is(err, missing):
			ttl = shopNegativeTTL
		default:
			return data, err
		}
		if encoded, marshalErr := json.Marshal(entry); marshalErr == nil {
			storeCtx, cancel := context.WithTimeout(ctx, shopCacheIO)
			defer cancel()
			_ = cacheStore.Run(storeCtx, c.redis, []string{key, key + ":generation", key + ":lock"}, generation, encoded, ttl.Milliseconds(), lease).Err()
		}
		return data, err
	}
	var entry shopCacheValue[T]
	if body := cacheString(values[0]); body != "" && json.Unmarshal([]byte(body), &entry) == nil && entry.RefreshAt > 0 && (entry.Data != nil || missing != nil) {
		if entry.Data == nil {
			var zero T
			return zero, missing
		}
		if time.Now().UnixMilli() >= entry.RefreshAt {
			c.refresh(key, generation, func(ctx context.Context, token string) { _, _ = reload(ctx, token) })
		}
		return *entry.Data, nil
	}
	return reload(ctx, "")
}

func (c *shopCache) invalidate(ctx context.Context, id int64) {
	key := shopDetailCacheKey(id)
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shopCacheIO)
	defer cancel()
	if err := cacheInvalidate.Run(ctx, c.redis, []string{key, key + ":generation"}, rand.Text(), (2 * time.Hour).Milliseconds()).Err(); err != nil {
		log.Printf("shop cache invalidation failed for shop %d", id)
	}
}

type cachedShopDetails struct {
	cache  *shopCache
	source shopReader
}

func (r cachedShopDetails) FindByID(ctx context.Context, id int64) (shopDetail, error) {
	return readShopCache(r.cache, ctx, shopDetailCacheKey(id), errShopNotFound, func(ctx context.Context) (shopDetail, error) {
		return r.source.FindByID(ctx, id)
	})
}

type cachedShopTypes struct {
	cache  *shopCache
	source shopTypeReader
}

func (r cachedShopTypes) ListShopTypes(ctx context.Context) ([]shopType, error) {
	return readShopCache(r.cache, ctx, "dianping:go:shop-types", nil, r.source.ListShopTypes)
}

type cachedShopWriter struct {
	cache  *shopCache
	source shopWriter
}

func (r cachedShopWriter) CreateShop(ctx context.Context, input shopWriteInput) (int64, error) {
	id, err := r.source.CreateShop(ctx, input)
	if err == nil {
		r.cache.invalidate(ctx, id)
	}
	return id, err
}

func (r cachedShopWriter) UpdateShop(ctx context.Context, id int64, input shopWriteInput) error {
	if err := r.source.UpdateShop(ctx, id, input); err != nil {
		return err
	}
	r.cache.invalidate(ctx, id)
	return nil
}
