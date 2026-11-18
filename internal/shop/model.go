package shop

import (
	"context"
	"errors"

	"github.com/Zhang02xin/hm-dianping-go/internal/localtime"
)

type shopDetail struct {
	ID         int64           `json:"id"`
	Name       string          `json:"name"`
	TypeID     *int64          `json:"typeId,omitempty"`
	Images     *string         `json:"images,omitempty"`
	Area       *string         `json:"area,omitempty"`
	Address    string          `json:"address"`
	X          *float64        `json:"x,omitempty"`
	Y          *float64        `json:"y,omitempty"`
	AvgPrice   *int64          `json:"avgPrice,omitempty"`
	Sold       *int            `json:"sold,omitempty"`
	Comments   *int            `json:"comments,omitempty"`
	Score      *int            `json:"score,omitempty"`
	OpenHours  *string         `json:"openHours,omitempty"`
	CreateTime *localtime.Time `json:"createTime,omitempty"`
	UpdateTime *localtime.Time `json:"updateTime,omitempty"`
	Distance   *float64        `json:"distance,omitempty"`
}

var shopDetailColumns = []string{
	"id", "name", "type_id", "images", "area", "address", "x", "y", "avg_price",
	"sold", "comments", "score", "open_hours", "create_time", "update_time",
}

var errShopNotFound = errors.New("shop not found")

type shopReader interface {
	FindByID(ctx context.Context, id int64) (shopDetail, error)
}
