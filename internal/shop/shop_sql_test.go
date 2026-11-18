package shop

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Zhang02xin/hm-dianping-go/internal/localtime"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestDatabaseShopReaderFindByID(t *testing.T) {
	const id int64 = 12
	columns := strings.Split("id,name,type_id,images,area,address,x,y,avg_price,sold,comments,score,open_hours,create_time,update_time", ",")
	databaseErr := errors.New("connection lost")
	createdAt := time.Date(2021, 12, 22, 20, 17, 47, 0, time.UTC)
	shop := shopDetail{
		ID: id, Name: "示例商铺", Address: "示例路 12 号", TypeID: new(int64(2)), Images: new("/a.jpg"),
		X: new(120.0), Y: new(30.0), AvgPrice: new(int64(0)), Sold: new(12), Comments: new(0),
		Score: new(45), OpenHours: new("08:00-22:00"), CreateTime: new(localtime.Time(createdAt)),
	}
	query := "SELECT " + strings.Join(columns, ", ") + " FROM tb_shop WHERE id = ?"
	tests := []struct {
		name    string
		found   bool
		dbErr   error
		want    shopDetail
		wantErr error
	}{
		{name: "found", found: true, want: shop},
		{name: "missing", wantErr: errShopNotFound},
		{name: "query failed", dbErr: databaseErr, wantErr: databaseErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })

			queryExpectation := mock.ExpectQuery(query).WithArgs(id)
			if tt.dbErr != nil {
				queryExpectation.WillReturnError(tt.dbErr)
			} else {
				rows := sqlmock.NewRows(columns)
				if tt.found {
					rows.AddRow(id, shop.Name, 2, "/a.jpg", nil, shop.Address, 120.0, 30.0, 0, 12, 0, 45, "08:00-22:00", createdAt, nil)
				}
				queryExpectation.WillReturnRows(rows).RowsWillBeClosed()
			}

			reader := sqlShopReader{db: db}
			got, err := reader.FindByID(context.Background(), id)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("shop = %+v, want %+v", got, tt.want)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestShopDetailJSON(t *testing.T) {
	createdAt := localtime.Time(time.Date(2021, 12, 22, 20, 17, 47, 0, time.FixedZone("CST", 8*3600)))
	shop := shopDetail{ID: 12, Name: "示例商铺", Address: "示例路 12 号", AvgPrice: new(int64(0)), CreateTime: &createdAt}
	got, err := json.Marshal(shop)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"id":12,"name":"示例商铺","address":"示例路 12 号","avgPrice":0,"createTime":"2021-12-22T20:17:47"}`
	if string(got) != want {
		t.Errorf("JSON = %s, want %s", got, want)
	}
}
