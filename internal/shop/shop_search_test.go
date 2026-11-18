package shop

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestShopSearchByName(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	columns := strings.Split("id,name,type_id,images,area,address,x,y,avg_price,sold,comments,score,open_hours,create_time,update_time", ",")
	query := "SELECT " + strings.Join(columns, ", ") + " FROM tb_shop"
	mock.ExpectQuery(query+" WHERE name LIKE ? ORDER BY id ASC LIMIT ? OFFSET ?").
		WithArgs("%O'Brien%", int64(10), int64(10)).WillReturnRows(sqlmock.NewRows(columns)).RowsWillBeClosed()
	mock.ExpectQuery(query+" ORDER BY id ASC LIMIT ? OFFSET ?").
		WithArgs(int64(10), int64(0)).WillReturnRows(sqlmock.NewRows(columns)).RowsWillBeClosed()

	router := testShopRouter(nil, nil, geoShopSearch{sqlShopReader: sqlShopReader{db: db}}, nil)
	for _, tt := range []struct {
		path   string
		status int
		body   string
	}{
		{"/shop/of/name?name=O%27Brien&current=2", http.StatusOK, `{"success":true,"data":[]}`},
		{"/shop/of/name", http.StatusOK, `{"success":true,"data":[]}`},
		{"/shop/of/name?current=0", http.StatusBadRequest, `{"success":false,"errorMsg":"invalid page"}`},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tt.path, nil))
		if response.Code != tt.status || response.Body.String() != tt.body {
			t.Errorf("%s response = %d %s, want %d %s", tt.path, response.Code, response.Body, tt.status, tt.body)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestShopSearchByType(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	columns := strings.Split("id,name,type_id,images,area,address,x,y,avg_price,sold,comments,score,open_hours,create_time,update_time", ",")
	selectColumns := "SELECT " + strings.Join(columns, ", ")
	mock.ExpectQuery(selectColumns+" FROM tb_shop WHERE type_id = ? ORDER BY id ASC LIMIT ? OFFSET ?").
		WithArgs(int64(2), int64(10), int64(10)).WillReturnRows(sqlmock.NewRows(columns))
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	if err := client.GeoAdd(context.Background(), "shop:geo:2",
		&redis.GeoLocation{Name: "12", Longitude: 121.48, Latitude: 31.23},
		&redis.GeoLocation{Name: "13", Longitude: 121.49, Latitude: 31.23},
		&redis.GeoLocation{Name: "14", Longitude: 122.48, Latitude: 31.23}).Err(); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(selectColumns+" FROM tb_shop WHERE type_id = ? AND id IN (?,?)").WithArgs(int64(2), int64(12), int64(13)).
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow(13, "较远商铺", 2, "", nil, "地址", 121.49, 31.23, nil, 0, 0, 0, nil, nil, nil).
			AddRow(12, "较近商铺", 2, "", nil, "地址", 121.48, 31.23, nil, 0, 0, 0, nil, nil, nil))
	router := testShopRouter(nil, nil, geoShopSearch{sqlShopReader: sqlShopReader{db: db}, cache: client}, nil)
	for _, tt := range []struct {
		path       string
		wantStatus int
		wantCount  int
	}{
		{"/shop/of/type?typeId=2&current=2", http.StatusOK, 0},
		{"/shop/of/type?typeId=2&x=121.48&y=31.23", http.StatusOK, 2},
		{"/shop/of/type?typeId=2&x=NaN&y=31.23", http.StatusBadRequest, 0},
		{"/shop/of/type", http.StatusBadRequest, 0},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tt.path, nil))
		var result struct {
			Success bool `json:"success"`
			Data    []struct {
				ID       int64    `json:"id"`
				Distance *float64 `json:"distance"`
			} `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if response.Code != tt.wantStatus || result.Success != (tt.wantStatus == http.StatusOK) || len(result.Data) != tt.wantCount {
			t.Fatalf("%s response = %d %s", tt.path, response.Code, response.Body)
		}
		if tt.wantCount == 2 && (result.Data[0].ID != 12 || result.Data[1].ID != 13 || result.Data[0].Distance == nil || result.Data[1].Distance == nil || *result.Data[0].Distance >= *result.Data[1].Distance) {
			t.Errorf("distance or ID not preserved: %s", response.Body)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
