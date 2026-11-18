package voucher

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Zhang02xin/hm-dianping-go/internal/user"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func TestVoucherRoutes(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := sqlVouchers{db: db}
	router := gin.New()
	client := testRedis(t)
	(&Service{repo: repo, loc: time.UTC, admission: redisVouchers{client: client}}).Register(router)
	const add = "INSERT INTO tb_voucher (shop_id, title, sub_title, rules, pay_value, actual_value, type, status) VALUES (?, ?, ?, ?, ?, ?, ?, ?)"
	const addStock = "INSERT INTO tb_seckill_voucher (voucher_id, stock, begin_time, end_time) VALUES (?, ?, ?, ?)"
	begin := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	end := begin.Add(2 * time.Hour)
	mock.ExpectBegin()
	mock.ExpectExec(add).WithArgs(int64(12), "普通券", nil, nil, int64(800), int64(1000), 0, 1).WillReturnResult(sqlmock.NewResult(21, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(add).WithArgs(int64(12), "秒杀券", nil, nil, int64(800), int64(1000), 1, 1).WillReturnResult(sqlmock.NewResult(22, 1))
	mock.ExpectExec(addStock).WithArgs(int64(22), 2, begin, end).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(add).WithArgs(int64(12), "秒杀券", nil, nil, int64(800), int64(1000), 1, 1).WillReturnResult(sqlmock.NewResult(23, 1))
	mock.ExpectExec(addStock).WithArgs(int64(23), 2, begin, end).WillReturnError(errors.New("stock insert failed"))
	mock.ExpectRollback()
	mock.ExpectQuery(listVouchersSQL).WithArgs(int64(12)).WillReturnRows(sqlmock.NewRows([]string{
		"id", "shop_id", "title", "sub_title", "rules", "pay_value", "actual_value", "type", "stock", "begin_time", "end_time",
	}).AddRow(21, 12, "普通券", nil, nil, 800, 1000, 0, nil, nil, nil).AddRow(22, 12, "秒杀券", nil, nil, 800, 1000, 1, 2, begin, end)).RowsWillBeClosed()
	const flashBody = `{"shopId":12,"title":"秒杀券","payValue":800,"actualValue":1000,"stock":2,"beginTime":"2026-11-18T10:00:00","endTime":"2026-11-18T12:00:00"}`
	for _, tt := range []struct {
		method, path, body, want string
		status                   int
	}{
		{http.MethodPost, "/voucher", `{"shopId":12,"title":"普通券","payValue":800,"actualValue":1000}`, `{"success":true,"data":21}`, 200},
		{http.MethodPost, "/voucher/seckill", flashBody, `{"success":true,"data":22}`, 200},
		{http.MethodPost, "/voucher/seckill", flashBody, `{"success":false,"errorMsg":"服务器异常"}`, 200},
		{http.MethodPost, "/voucher/seckill", strings.Replace(flashBody, "T12:00:00", "T09:00:00", 1), `{"success":false,"errorMsg":"秒杀时间或库存无效"}`, 400},
		{http.MethodGet, "/voucher/list/12", "", `{"success":true,"data":[{"id":21,"shopId":12,"title":"普通券","payValue":800,"actualValue":1000,"type":0},{"id":22,"shopId":12,"title":"秒杀券","payValue":800,"actualValue":1000,"type":1,"stock":2,"beginTime":"2026-11-18T10:00:00","endTime":"2026-11-18T12:00:00"}]}`, 200},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body)))
		if response.Code != tt.status || response.Body.String() != tt.want {
			t.Errorf("%s %s response = %d %s, want %d %s", tt.method, tt.path, response.Code, response.Body, tt.status, tt.want)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
	if stock := client.Get(context.Background(), stockKey(22)).Val(); stock != "2" {
		t.Errorf("Redis stock = %s", stock)
	}
	if client.Exists(context.Background(), stockKey(23)).Val() != 0 {
		t.Error("rolled-back coupon created Redis stock")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectExec(add).WithArgs(int64(12), "秒杀券", nil, nil, int64(800), int64(1000), 1, 1).WillReturnResult(sqlmock.NewResult(24, 1))
	mock.ExpectExec(addStock).WithArgs(int64(24), 2, begin, end).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/voucher/seckill", strings.NewReader(flashBody)))
	if response.Body.String() != `{"success":false,"errorMsg":"服务器异常"}` {
		t.Fatalf("stock initialization failure response = %s", response.Body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func testRedis(t *testing.T) *redis.Client {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), ContextTimeoutEnabled: true})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestVoucherPurchaseRoutes(t *testing.T) {
	client := testRedis(t)
	ctx := context.Background()
	const orderID, userID, voucherID int64 = 103, 42, 22
	admission := redisVouchers{client: client}
	if err := admission.setStock(ctx, voucherID, 2); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if c.GetHeader("X-Test-User") != "" {
			user.SetCurrent(c, user.Summary{ID: userID})
		}
	})
	(&Service{nextID: func(context.Context) (int64, error) { return orderID, nil }, admission: admission}).Register(router)
	for _, tt := range []struct {
		path, token string
		status      int
		want        string
	}{
		{"/voucher-order/seckill/22", "", 401, ""},
		{"/voucher-order/seckill/22", "1", 200, `{"success":true,"data":103}`},
		{"/voucher-order/seckill/22", "1", 200, `{"success":false,"errorMsg":"禁止重复下单"}`},
		{"/voucher-order/seckill/99", "1", 200, `{"success":false,"errorMsg":"库存不足"}`},
	} {
		req := httptest.NewRequest(http.MethodPost, tt.path, nil)
		req.Header.Set("X-Test-User", tt.token)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != tt.status || res.Body.String() != tt.want {
			t.Fatalf("response = %d %s", res.Code, res.Body)
		}
	}
	entries, err := client.XRange(ctx, orderStream, "-", "+").Result()
	if err != nil || len(entries) != 1 || entries[0].Values["id"] != "103" || entries[0].Values["voucherId"] != "22" || entries[0].Values["userId"] != "42" {
		t.Fatalf("legacy stream entries: %+v %v", entries, err)
	}
	if remaining := client.Get(ctx, stockKey(voucherID)).Val(); remaining != "1" {
		t.Errorf("remaining Redis stock = %s", remaining)
	}
	if !client.SIsMember(ctx, buyersKey(voucherID), "42").Val() {
		t.Error("buyer missing from original Redis set")
	}
}
