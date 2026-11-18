package social

import "github.com/Zhang02xin/hm-dianping-go/internal/localtime"

const blogPageSize = 10

type blog struct {
	ID         int64           `json:"id"`
	ShopID     int64           `json:"shopId"`
	UserID     int64           `json:"userId"`
	Icon       *string         `json:"icon,omitempty"`
	Name       *string         `json:"name,omitempty"`
	IsLike     *bool           `json:"isLike,omitempty"`
	Title      string          `json:"title"`
	Images     string          `json:"images"`
	Content    string          `json:"content"`
	Liked      *int64          `json:"liked,omitempty"`
	Comments   *int64          `json:"comments,omitempty"`
	CreateTime *localtime.Time `json:"createTime,omitempty"`
	UpdateTime *localtime.Time `json:"updateTime,omitempty"`
}

type blogInput struct {
	ShopID  int64   `json:"shopId"`
	Title   string  `json:"title"`
	Images  *string `json:"images"`
	Content *string `json:"content"`
}

type blogScroll struct {
	List    []blog `json:"list"`
	MinTime int64  `json:"minTime"`
	Offset  int64  `json:"offset"`
}
