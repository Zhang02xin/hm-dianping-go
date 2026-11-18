package shop

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestShopTypeList(t *testing.T) {
	columns := []string{"id", "name", "icon", "sort"}
	tests := []struct {
		name string
		rows *sqlmock.Rows
		err  error
		want string
	}{
		{"found", sqlmock.NewRows(columns).AddRow(3, "美食", nil, 0).AddRow(1, "KTV", "/types/KTV.png", 2), nil,
			`{"success":true,"data":[{"id":3,"name":"美食","sort":0},{"id":1,"name":"KTV","icon":"/types/KTV.png","sort":2}]}`},
		{"empty", sqlmock.NewRows(columns), nil, `{"success":true,"data":[]}`},
		{"query failed", nil, errors.New("connection lost"), `{"success":false,"errorMsg":"服务器异常"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			query := mock.ExpectQuery("SELECT id, name, icon, sort FROM tb_shop_type ORDER BY sort ASC, id ASC")
			if tt.err != nil {
				query.WillReturnError(tt.err)
			} else {
				query.WillReturnRows(tt.rows).RowsWillBeClosed()
			}
			router := testShopRouter(nil, sqlShopTypeReader{db: db}, nil, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/shop-type/list", nil))
			if response.Code != http.StatusOK || response.Body.String() != tt.want {
				t.Errorf("response = %d %s, want 200 %s", response.Code, response.Body, tt.want)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Error(err)
			}
		})
	}
}
