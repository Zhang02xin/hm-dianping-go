package shop

import (
	"context"
	"database/sql"
	"time"
)

type shopType struct {
	ID   int64   `json:"id"`
	Name *string `json:"name,omitempty"`
	Icon *string `json:"icon,omitempty"`
	Sort *int    `json:"sort,omitempty"`
}

type shopTypeReader interface {
	ListShopTypes(context.Context) ([]shopType, error)
}

type sqlShopTypeReader struct {
	db *sql.DB
}

func (r sqlShopTypeReader) ListShopTypes(ctx context.Context) ([]shopType, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	rows, err := r.db.QueryContext(ctx, "SELECT id, name, icon, sort FROM tb_shop_type ORDER BY sort ASC, id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	types := make([]shopType, 0)
	for rows.Next() {
		var item shopType
		if err := rows.Scan(&item.ID, &item.Name, &item.Icon, &item.Sort); err != nil {
			return nil, err
		}
		types = append(types, item)
	}
	return types, rows.Err()
}
