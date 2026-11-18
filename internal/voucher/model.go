package voucher

import (
	"time"

	"github.com/Zhang02xin/hm-dianping-go/internal/localtime"
)

type voucher struct {
	ID          int64           `json:"id"`
	ShopID      *int64          `json:"shopId,omitempty"`
	Title       string          `json:"title"`
	SubTitle    *string         `json:"subTitle,omitempty"`
	Rules       *string         `json:"rules,omitempty"`
	PayValue    int64           `json:"payValue"`
	ActualValue int64           `json:"actualValue"`
	Type        int             `json:"type"`
	Stock       *int            `json:"stock,omitempty"`
	BeginTime   *localtime.Time `json:"beginTime,omitempty"`
	EndTime     *localtime.Time `json:"endTime,omitempty"`
}

type voucherInput struct {
	ShopID      *int64  `json:"shopId"`
	Title       string  `json:"title"`
	SubTitle    *string `json:"subTitle"`
	Rules       *string `json:"rules"`
	PayValue    *int64  `json:"payValue"`
	ActualValue *int64  `json:"actualValue"`
	Status      *int    `json:"status"`
	Stock       *int    `json:"stock"`
	BeginTime   string  `json:"beginTime"`
	EndTime     string  `json:"endTime"`
}

type voucherWindow struct {
	stock      int
	begin, end time.Time
}
