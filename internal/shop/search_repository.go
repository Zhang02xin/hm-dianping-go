package shop

import (
	"context"
	"strings"
	"time"
)

const (
	shopPageSize = 10
	shopRadiusM  = 5000
)

type shopSearchReader interface {
	FindShopsByName(context.Context, string, int64) ([]shopDetail, error)
	FindShopsByType(context.Context, int64, *shopCoordinates, int64) ([]shopDetail, error)
}

type shopCoordinates struct{ x, y float64 }

func (r sqlShopReader) FindShopsByName(ctx context.Context, name string, offset int64) ([]shopDetail, error) {
	query := "SELECT " + strings.Join(shopDetailColumns, ", ") + " FROM tb_shop"
	var args []any
	if strings.TrimSpace(name) != "" {
		query += " WHERE name LIKE ?"
		args = append(args, "%"+name+"%")
	}
	return r.queryShops(ctx, query+" ORDER BY id ASC LIMIT ? OFFSET ?", append(args, shopPageSize, offset)...)
}

func (r sqlShopReader) FindShopsByType(ctx context.Context, typeID, offset int64) ([]shopDetail, error) {
	columns := strings.Join(shopDetailColumns, ", ")
	return r.queryShops(ctx, "SELECT "+columns+" FROM tb_shop WHERE type_id = ? ORDER BY id ASC LIMIT ? OFFSET ?", typeID, shopPageSize, offset)
}

func (r sqlShopReader) queryShops(ctx context.Context, query string, args ...any) ([]shopDetail, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	shops := make([]shopDetail, 0)
	for rows.Next() {
		shop, err := scanShopDetail(rows)
		if err != nil {
			return nil, err
		}
		shops = append(shops, shop)
	}
	return shops, rows.Err()
}
