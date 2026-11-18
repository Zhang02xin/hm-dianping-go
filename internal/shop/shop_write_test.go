package shop

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestShopWrites(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectExec("INSERT INTO tb_shop (name, type_id, images, address, x, y, sold, comments, score) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)").
		WithArgs("新商铺", int64(1), "/a.jpg", "地址", 121.48, 31.23, int64(0), int64(0), int64(0)).WillReturnResult(sqlmock.NewResult(15, 1))
	mock.ExpectExec("UPDATE tb_shop SET avg_price = ? WHERE id = ?").
		WithArgs(int64(0), int64(15)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE tb_shop SET name = ? WHERE id = ?").
		WithArgs("新名", int64(15)).WillReturnError(errors.New("connection lost"))
	router := testShopRouter(nil, nil, nil, sqlShopReader{db: db})
	for _, tt := range []struct {
		method string
		body   string
		status int
		want   string
	}{
		{http.MethodPost, `{"name":"新商铺","typeId":1,"images":"/a.jpg","address":"地址","x":121.48,"y":31.23}`, http.StatusOK, `{"success":true}`},
		{http.MethodPut, `{"id":15,"avgPrice":0,"address":null}`, http.StatusOK, `{"success":true}`},
		{http.MethodPut, `{"id":15,"name":"新名"}`, http.StatusOK, `{"success":false,"errorMsg":"服务器异常"}`},
		{http.MethodPut, `{"name":"新名"}`, http.StatusOK, `{"success":false,"errorMsg":"id不能为空"}`},
		{http.MethodPost, `{}`, http.StatusBadRequest, `{"success":false,"errorMsg":"商铺必填字段缺失"}`},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(tt.method, "/shop", strings.NewReader(tt.body)))
		if response.Code != tt.status || response.Body.String() != tt.want {
			t.Errorf("%s %s response = %d %s, want %d %s", tt.method, tt.body, response.Code, response.Body, tt.status, tt.want)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
