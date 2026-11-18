package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestApplicationRouteWiring(t *testing.T) {
	app := newApplication(nil, time.UTC, nil, nil, nil)
	t.Cleanup(app.shops.Close)
	registered := make(map[string]bool)
	for _, route := range app.router.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{
		"POST /user/login", "GET /shop/:id", "GET /shop-type/list", "GET /voucher/list/:shopId",
		"POST /voucher-order/seckill/:id", "GET /blog/hot", "PUT /follow/:id/:isFollow", "POST /upload/blog",
	} {
		if !registered[route] {
			t.Errorf("application is missing %s", route)
		}
	}
	for _, tt := range []struct {
		path string
		code int
	}{
		{"/ping", http.StatusOK},
		{"/user/me", http.StatusUnauthorized},
		{"/shops/12", http.StatusNotFound},
	} {
		w := httptest.NewRecorder()
		app.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tt.path, nil))
		if w.Code != tt.code {
			t.Errorf("%s status = %d, want %d", tt.path, w.Code, tt.code)
		}
	}
}
