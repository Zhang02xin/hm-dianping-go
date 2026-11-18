package user

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type captureCodeSender struct{ phone, code string }

func (s *captureCodeSender) SendCode(_ context.Context, phone, code string) error {
	s.phone, s.code = phone, code
	return nil
}

func TestUserAuthFlow(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const phone = "13800138000"
	const lookup = "SELECT id, nick_name, icon FROM tb_user WHERE phone = ?"
	columns := []string{"id", "nick_name", "icon"}
	mock.ExpectQuery(lookup).WithArgs(phone).WillReturnRows(sqlmock.NewRows(columns))
	mock.ExpectExec("INSERT INTO tb_user (phone, nick_name) VALUES (?, ?)").WithArgs(phone, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(42, 1))
	mock.ExpectQuery(lookup).WithArgs(phone).WillReturnRows(sqlmock.NewRows(columns).AddRow(42, "测试用户", nil))
	mock.ExpectQuery("SELECT id, nick_name, icon FROM tb_user WHERE id = ?").WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows(columns).AddRow(42, "测试用户", nil))
	mock.ExpectQuery("SELECT user_id, city, introduce, fans, followee, gender, birthday, credits, level FROM tb_user_info WHERE user_id = ?").
		WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"user_id", "city", "introduce", "fans", "followee", "gender", "birthday", "credits", "level"}).
		AddRow(42, "上海", nil, 0, 0, 0, time.Date(2000, 3, 7, 0, 0, 0, 0, time.UTC), 0, 3))
	sender := &captureCodeSender{}
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	auth := &Service{sessions: redisSessions{client: client}, users: sqlUsers{db: db}, sender: sender, now: func() time.Time { return now }}
	router := gin.New()
	router.Use(auth.RefreshSession)
	Register(router, auth)
	call := func(method, path, body, token string, status int) string {
		t.Helper()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Authorization", token)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != status {
			t.Fatalf("%s %s status = %d, want %d", method, path, response.Code, status)
		}
		return response.Body.String()
	}
	requireBody := func(got, want string) {
		t.Helper()
		if got != want {
			t.Errorf("response does not match expected body %s", want)
		}
	}
	const ok = `{"success":true}`
	call(http.MethodGet, "/user/me", "", "", http.StatusUnauthorized)
	requireBody(call(http.MethodPost, "/user/code?phone="+phone, "", "", http.StatusOK), ok)
	if sender.phone != phone || !codePattern.MatchString(sender.code) || server.TTL("login:code:"+phone) != loginCodeTTL {
		t.Fatal("issued code or expiry does not match the contract")
	}
	requireBody(call(http.MethodPost, "/user/code?phone="+phone, "", "", http.StatusOK), `{"success":false,"errorMsg":"请稍后再发送验证码"}`)
	loginBody := `{"phone":"` + phone + `","code":"` + sender.code + `"}`
	var result struct {
		Success bool   `json:"success"`
		Data    string `json:"data"`
	}
	if err := json.Unmarshal([]byte(call(http.MethodPost, "/user/login", loginBody, "", http.StatusOK)), &result); err != nil || !result.Success || len(result.Data) != 32 {
		t.Fatal("login did not issue a valid session")
	}
	token := result.Data
	requireBody(call(http.MethodPost, "/user/login", loginBody, "", http.StatusOK), `{"success":false,"errorMsg":"验证码错误"}`)
	server.FastForward(20 * time.Minute)
	const summary = `{"success":true,"data":{"id":42,"nickName":"测试用户"}}`
	requireBody(call(http.MethodGet, "/user/me", "", token, http.StatusOK), summary)
	if server.TTL("login:token:"+token) != loginUserTTL {
		t.Error("session TTL was not refreshed")
	}
	requireBody(call(http.MethodGet, "/user/42", "", "Bearer "+token, http.StatusOK), summary)
	requireBody(call(http.MethodGet, "/user/info/42", "", token, http.StatusOK), `{"success":true,"data":{"userId":42,"city":"上海","fans":0,"followee":0,"gender":false,"birthday":"2000-03-07","credits":0,"level":true}}`)
	requireBody(call(http.MethodPost, "/user/sign", "", token, http.StatusOK), ok)
	now = now.AddDate(0, 0, 1)
	requireBody(call(http.MethodPost, "/user/sign", "", token, http.StatusOK), ok)
	requireBody(call(http.MethodGet, "/user/sign/count", "", token, http.StatusOK), `{"success":true,"data":2}`)
	now = now.AddDate(0, 0, 1)
	requireBody(call(http.MethodGet, "/user/sign/count", "", token, http.StatusOK), `{"success":true,"data":0}`)
	requireBody(call(http.MethodPost, "/user/logout", "", token, http.StatusOK), ok)
	call(http.MethodGet, "/user/me", "", token, http.StatusUnauthorized)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestFailedCodeAttemptsInvalidateCode(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	const phone = "13800138000"
	auth := &Service{sessions: redisSessions{client: client}, sender: &captureCodeSender{}}
	if err := auth.issueCode(ctx, phone); err != nil {
		t.Fatal(err)
	}
	valid := auth.sender.(*captureCodeSender).code
	wrong := "000000"
	if valid == wrong {
		wrong = "111111"
	}
	for i := range 5 {
		if _, err := auth.login(ctx, phone, wrong); err != errInvalidCode {
			t.Fatalf("attempt %d did not reject the wrong code", i+1)
		}
	}
	if _, err := auth.login(ctx, phone, valid); err != errInvalidCode {
		t.Fatal("code still usable after five failed attempts")
	}
	if err := auth.issueCode(ctx, phone); err != errCodePending {
		t.Fatal("cooldown was cleared before the original expiry")
	}
	server.FastForward(loginCodeTTL)
	if err := auth.issueCode(ctx, phone); err != nil {
		t.Fatal("code cannot be reissued after expiry")
	}
}
