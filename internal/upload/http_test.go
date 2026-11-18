package upload

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"

	"github.com/Zhang02xin/hm-dianping-go/internal/user"

	"github.com/gin-gonic/gin"
)

func TestBlogImageLifecycle(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if userID, err := strconv.ParseInt(c.GetHeader("X-Test-User"), 10, 64); err == nil {
			user.SetCurrent(c, user.Summary{ID: userID})
		}
	})
	Register(router, root)
	var original bytes.Buffer
	if err := png.Encode(&original, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	upload := func(data []byte, user string) *httptest.ResponseRecorder {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("file", "wrong-extension.txt")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(data); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, "/upload/blog", &body)
		request.Header.Set("Content-Type", writer.FormDataContentType())
		request.Header.Set("X-Test-User", user)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	if got := upload(original.Bytes(), "").Code; got != http.StatusUnauthorized {
		t.Fatalf("anonymous upload status = %d", got)
	}
	if got := upload([]byte("not an image"), "21").Code; got != http.StatusBadRequest {
		t.Fatalf("non-image upload status = %d", got)
	}
	if got := upload(make([]byte, maxBlogImage+1), "21").Code; got != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized upload status = %d", got)
	}
	response := upload(original.Bytes(), "21")
	var result struct {
		Success bool
		Data    string
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || response.Code != 200 || !result.Success || !blogImagePath.MatchString(result.Data) {
		t.Fatalf("upload response = %d %s", response.Code, response.Body)
	}
	get := func(path, user string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("X-Test-User", user)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	servePath := "/imgs" + result.Data
	if served := get(servePath, ""); served.Code != 200 || served.Header().Get("Content-Type") != "image/png" || !bytes.Equal(served.Body.Bytes(), original.Bytes()) {
		t.Fatal("saved image differs from upload")
	}
	if code := get(servePath+".owner", "").Code; code != http.StatusNotFound {
		t.Fatalf("owner metadata exposed with status %d", code)
	}
	deletePath := "/upload/blog/delete?name=" + url.QueryEscape(result.Data)
	if code := get(deletePath, "99").Code; code != http.StatusForbidden {
		t.Fatalf("non-owner deletion status = %d", code)
	}
	if got := get("/upload/blog/delete?name="+url.QueryEscape("/../outside.png"), "21").Body.String(); got != `{"success":false,"errorMsg":"错误的文件名称"}` {
		t.Fatalf("out-of-root deletion response = %s", got)
	}
	if got := get(deletePath, "21").Body.String(); got != `{"success":true}` {
		t.Fatalf("owner deletion response = %s", got)
	}
	if code := get(servePath, "").Code; code != http.StatusNotFound {
		t.Fatalf("deleted image remains accessible with status %d", code)
	}
}
