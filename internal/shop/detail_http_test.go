package shop

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestShopDetail(t *testing.T) {
	reader := shopReaderFunc(func(_ context.Context, id int64) (shopDetail, error) {
		if id != 12 {
			return shopDetail{}, errShopNotFound
		}
		return shopDetail{ID: id, Name: "示例商铺", Address: "示例路 12 号"}, nil
	})
	router := testShopRouter(reader, nil, nil, nil)
	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   map[string]any
	}{
		{
			name: "found", path: "/shop/12", wantStatus: http.StatusOK,
			wantBody: map[string]any{
				"success": true,
				"data":    map[string]any{"id": float64(12), "name": "示例商铺", "address": "示例路 12 号"},
			},
		},
		{
			name: "not found", path: "/shop/99", wantStatus: http.StatusOK,
			wantBody: map[string]any{"success": false, "errorMsg": "店铺不存在"},
		},
		{
			name: "invalid ID", path: "/shop/abc", wantStatus: http.StatusBadRequest,
			wantBody: map[string]any{"success": false, "errorMsg": "invalid shop id"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", response.Code, tt.wantStatus)
			}
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}
			if !reflect.DeepEqual(body, tt.wantBody) {
				t.Errorf("body = %s, want %v", response.Body.String(), tt.wantBody)
			}
		})
	}
}

type failingShopReader struct {
	contextErr error
}

func (r *failingShopReader) FindByID(ctx context.Context, _ int64) (shopDetail, error) {
	r.contextErr = ctx.Err()
	return shopDetail{}, errors.New("storage unavailable")
}

func TestShopDetailReadFailure(t *testing.T) {
	reader := &failingShopReader{}
	router := testShopRouter(reader, nil, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/shop/12", nil).WithContext(ctx)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if !errors.Is(reader.contextErr, context.Canceled) {
		t.Errorf("reader context error = %v, want cancellation", reader.contextErr)
	}
	if response.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	want := map[string]any{"success": false, "errorMsg": "服务器异常"}
	if !reflect.DeepEqual(body, want) {
		t.Errorf("body = %s, want %v", response.Body.String(), want)
	}
}
