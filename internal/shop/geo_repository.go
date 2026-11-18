package shop

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type geoShopSearch struct {
	sqlShopReader
	cache *redis.Client
}

func (r geoShopSearch) FindShopsByType(ctx context.Context, typeID int64, point *shopCoordinates, offset int64) ([]shopDetail, error) {
	if point == nil {
		return r.sqlShopReader.FindShopsByType(ctx, typeID, offset)
	}
	if r.cache == nil {
		return nil, errors.New("shop geo index unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	locations, err := r.cache.GeoSearchLocation(ctx, "shop:geo:"+strconv.FormatInt(typeID, 10), &redis.GeoSearchLocationQuery{
		GeoSearchQuery: redis.GeoSearchQuery{Longitude: point.x, Latitude: point.y, Radius: shopRadiusM, RadiusUnit: "m", Sort: "ASC", Count: int(offset + shopPageSize)},
		WithDist:       true,
	}).Result()
	if err != nil {
		return nil, err
	}
	if int64(len(locations)) <= offset {
		return []shopDetail{}, nil
	}
	locations = locations[offset:]
	args := make([]any, 0, len(locations)+1)
	args = append(args, typeID)
	ids := make([]int64, 0, len(locations))
	for _, location := range locations {
		id, err := strconv.ParseInt(location.Name, 10, 64)
		if err != nil {
			return nil, err
		}
		args = append(args, id)
		ids = append(ids, id)
	}
	query := "SELECT " + strings.Join(shopDetailColumns, ", ") + " FROM tb_shop WHERE type_id = ? AND id IN (" + strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",") + ")"
	rows, err := r.queryShops(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]shopDetail, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	result := make([]shopDetail, 0, len(rows))
	for i, id := range ids {
		if row, ok := byID[id]; ok {
			row.Distance = new(locations[i].Dist)
			result = append(result, row)
		}
	}
	return result, nil
}
