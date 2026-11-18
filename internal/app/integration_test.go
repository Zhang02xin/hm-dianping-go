package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Zhang02xin/hm-dianping-go/internal/user"

	"github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
)

func TestIntegrationLoginAndAsyncOrder(t *testing.T) {
	dsn, redisURL := os.Getenv("DIANPING_TEST_MYSQL_DSN"), os.Getenv("DIANPING_TEST_REDIS_URL")
	if dsn == "" && redisURL == "" {
		t.Skip("requires disposable MySQL and Redis instances; see README")
	}
	config, err := mysql.ParseDSN(dsn)
	if err != nil || !strings.HasPrefix(config.DBName, "hmdp_test_") || redisURL == "" {
		t.Fatal("configure a disposable hmdp_test_* database and dedicated Redis instance")
	}
	config.ParseTime = true
	connector, err := mysql.NewConnector(config)
	if err != nil {
		t.Fatal("cannot configure integration MySQL")
	}
	db := sql.OpenDB(connector)
	t.Cleanup(func() { _ = db.Close() })
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal("cannot configure integration Redis")
	}
	options.ContextTimeoutEnabled = true
	cacheClient := redis.NewClient(options)
	t.Cleanup(func() { _ = cacheClient.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal("integration MySQL is unavailable")
	}
	if err := cacheClient.Ping(ctx).Err(); err != nil {
		t.Fatal("integration Redis is unavailable")
	}
	type deliveredCode struct{ Phone, Code string }
	delivered := make(chan deliveredCode, 1)
	sms := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data deliveredCode
		if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&data) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		select {
		case delivered <- data:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusConflict)
		}
	}))
	t.Cleanup(sms.Close)
	sender, err := user.NewCodeSender(sms.URL, sms.Client().Transport)
	if err != nil {
		t.Fatal(err)
	}
	app := newApplication(db, config.Loc, cacheClient, sender, nil)
	t.Cleanup(app.shops.Close)
	router := app.router
	stop, err := app.vouchers.StartWorker(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)

	type responseBody struct {
		Success  bool            `json:"success"`
		Data     json.RawMessage `json:"data"`
		ErrorMsg string          `json:"errorMsg"`
	}
	call := func(method, route, data, token string) responseBody {
		t.Helper()
		req := httptest.NewRequest(method, route, strings.NewReader(data)).WithContext(ctx)
		req.Header.Set("Authorization", token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		var response responseBody
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &response) != nil {
			t.Fatalf("%s %s returned HTTP %d or invalid JSON", method, strings.SplitN(route, "?", 2)[0], w.Code)
		}
		return response
	}
	if r := call(http.MethodGet, "/shop/1", "", ""); !r.Success || len(r.Data) == 0 {
		t.Fatal("seeded shop 1 is unavailable")
	}
	number, err := rand.Int(rand.Reader, big.NewInt(100000000))
	if err != nil {
		t.Fatal(err)
	}
	phone := fmt.Sprintf("138%08d", number.Int64())
	if !call(http.MethodPost, "/user/code?phone="+phone, "", "").Success {
		t.Fatal("failed to issue integration code")
	}
	var code deliveredCode
	select {
	case code = <-delivered:
	case <-ctx.Done():
		t.Fatal("test gateway did not receive the code")
	}
	if code.Phone != phone || len(code.Code) != 6 {
		t.Fatal("test gateway received an invalid code payload")
	}
	login := call(http.MethodPost, "/user/login", fmt.Sprintf(`{"phone":%q,"code":%q}`, phone, code.Code), "")
	var token string
	if !login.Success || json.Unmarshal(login.Data, &token) != nil || token == "" {
		t.Fatal("login did not issue a token")
	}
	var account user.Summary
	me := call(http.MethodGet, "/user/me", "", token)
	if !me.Success || json.Unmarshal(me.Data, &account) != nil || account.ID <= 0 {
		t.Fatal("issued session could not resolve the user")
	}
	// Use the same database clock that evaluates the active sale window.
	var begin, end string
	if err := db.QueryRowContext(ctx, `SELECT DATE_FORMAT(CURRENT_TIMESTAMP - INTERVAL 1 MINUTE, '%Y-%m-%dT%H:%i:%s'),
DATE_FORMAT(CURRENT_TIMESTAMP + INTERVAL 3 MINUTE, '%Y-%m-%dT%H:%i:%s')`).Scan(&begin, &end); err != nil {
		t.Fatal("could not derive integration sale window")
	}
	v := call(http.MethodPost, "/voucher/seckill", fmt.Sprintf(`{"shopId":1,"title":"集成测试券","payValue":100,"actualValue":200,"stock":2,"beginTime":%q,"endTime":%q}`, begin, end), "")
	var voucherID int64
	if !v.Success || json.Unmarshal(v.Data, &voucherID) != nil || voucherID <= 0 {
		t.Fatal("could not create the integration voucher")
	}
	orderPath := fmt.Sprintf("/voucher-order/seckill/%d", voucherID)
	order := call(http.MethodPost, orderPath, "", token)
	var orderID int64
	if !order.Success || json.Unmarshal(order.Data, &orderID) != nil || orderID <= 0 {
		t.Fatal("could not reserve the integration order")
	}
	if second := call(http.MethodPost, orderPath, "", token); second.Success || second.ErrorMsg != "禁止重复下单" {
		t.Fatal("repeated purchase did not return the duplicate rejection")
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		var count, stock int
		if err := db.QueryRowContext(ctx, `SELECT
(SELECT COUNT(*) FROM tb_voucher_order WHERE id = ? AND user_id = ? AND voucher_id = ?),
(SELECT stock FROM tb_seckill_voucher WHERE voucher_id = ?)`, orderID, account.ID, voucherID, voucherID).Scan(&count, &stock); err != nil {
			t.Fatal("could not read durable order and stock")
		}
		if stock != 1 {
			t.Fatalf("expected exactly one reservation, stock = %d", stock)
		}
		if count == 1 {
			break
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("accepted order was not finalized")
		}
	}
}
