package shop

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type sqlShopReader struct {
	db *sql.DB
}

func (r sqlShopReader) FindByID(ctx context.Context, id int64) (shopDetail, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	query := "SELECT " + strings.Join(shopDetailColumns, ", ") + " FROM tb_shop WHERE id = ?"
	shop, err := scanShopDetail(r.db.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return shopDetail{}, errShopNotFound
	}
	if err != nil {
		return shopDetail{}, fmt.Errorf("query shop %d: %w", id, err)
	}
	return shop, nil
}

func scanShopDetail(row interface{ Scan(...any) error }) (shopDetail, error) {
	var shop shopDetail
	dest := []any{
		&shop.ID, &shop.Name, &shop.TypeID, &shop.Images, &shop.Area, &shop.Address,
		&shop.X, &shop.Y, &shop.AvgPrice, &shop.Sold, &shop.Comments, &shop.Score,
		&shop.OpenHours, &shop.CreateTime, &shop.UpdateTime,
	}
	err := row.Scan(dest...)
	return shop, err
}
