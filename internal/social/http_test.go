package social

import (
	"context"
	"encoding/json"
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

func socialTestRouter(t *testing.T) (*gin.Engine, sqlmock.Sqlmock, *redis.Client) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if c.GetHeader("X-Test-User") == "1" {
			user.SetCurrent(c, user.Summary{ID: 42})
		}
	})
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	Register(router, db, client)
	return router, mock, client
}

func callSocial(t *testing.T, router *gin.Engine, method, path, body string, loggedIn bool, status int) string {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if loggedIn {
		req.Header.Set("X-Test-User", "1")
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != status {
		t.Fatalf("%s %s status = %d, want %d; body %s", method, path, w.Code, status, w.Body)
	}
	return w.Body.String()
}

func TestBlogWriteAndLike(t *testing.T) {
	router, mock, client := socialTestRouter(t)
	const input = `{"shopId":3,"userId":900,"title":"探店","images":"/a.png","content":"正文"}`
	const ok = `{"success":true}`
	const failure = `{"success":false,"errorMsg":"服务器异常"}`
	callSocial(t, router, http.MethodPost, "/blog", input, false, 401)
	ctx := context.Background()
	mock.ExpectQuery("SELECT DISTINCT user_id FROM tb_follow WHERE follow_user_id = ?").WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(43))
	mock.ExpectExec("INSERT INTO tb_blog (shop_id, user_id, title, images, content) VALUES (?, ?, ?, ?, ?)").
		WithArgs(int64(3), int64(42), "探店", "/a.png", "正文").WillReturnResult(sqlmock.NewResult(21, 1))
	if got := callSocial(t, router, http.MethodPost, "/blog", input, true, 200); got != `{"success":true,"data":21}` {
		t.Errorf("create response = %s", got)
	}
	if got := client.ZRange(ctx, "feed:43", 0, -1).Val(); len(got) != 1 || got[0] != "21" {
		t.Errorf("feed = %v", got)
	}
	for _, removed := range []int64{0, 1} {
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT id FROM tb_blog WHERE id = ? FOR UPDATE").WithArgs(int64(21)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(21))
		want := ok
		if removed == 0 {
			mock.ExpectExec("UPDATE tb_blog SET liked = COALESCE(liked, 0) + 1 WHERE id = ?").WithArgs(int64(21)).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
		} else {
			mock.ExpectExec("UPDATE tb_blog SET liked = GREATEST(CAST(COALESCE(liked, 0) AS SIGNED) - 1, 0) WHERE id = ?").WithArgs(int64(21)).WillReturnError(errors.New("counter failed"))
			mock.ExpectRollback()
			want = failure
		}
		if got := callSocial(t, router, http.MethodPut, "/blog/like/21", "", true, 200); got != want {
			t.Errorf("toggle response = %s, want %s", got, want)
		}
	}
	if _, err := client.ZScore(ctx, "blog:liked:21", "42").Result(); err != nil {
		t.Fatal("successful Redis like did not survive failed unlike")
	}
	mock.ExpectQuery("SELECT id, nick_name, icon FROM tb_user WHERE id IN (?)").WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "nick_name", "icon"}).AddRow(42, "读者", nil))
	if got := callSocial(t, router, http.MethodGet, "/blog/likes/21", "", true, 200); !strings.Contains(got, `"id":42`) {
		t.Errorf("likers = %s", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestFollowAndFeedCursor(t *testing.T) {
	router, mock, client := socialTestRouter(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM tb_user WHERE id = ? FOR UPDATE").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
	mock.ExpectExec("INSERT INTO tb_follow (user_id, follow_user_id) SELECT ?, ? FROM DUAL WHERE NOT EXISTS(SELECT 1 FROM tb_follow WHERE user_id = ? AND follow_user_id = ?)").
		WithArgs(int64(42), int64(9), int64(42), int64(9)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	if got := callSocial(t, router, http.MethodPut, "/follow/9/true", "", true, 200); got != `{"success":true}` {
		t.Errorf("follow response = %s", got)
	}
	mock.ExpectQuery("SELECT EXISTS(SELECT 1 FROM tb_follow WHERE user_id = ? AND follow_user_id = ?)").WithArgs(int64(42), int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	if got := callSocial(t, router, http.MethodGet, "/follow/or/not/9", "", true, 200); got != `{"success":true,"data":false}` {
		t.Errorf("following response = %s", got)
	}
	ctx := context.Background()
	if !client.SIsMember(ctx, "follows:42", "9").Val() {
		t.Fatal("Redis follows set missing member")
	}
	if err := client.SAdd(ctx, "follows:87", "9", "10").Err(); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT id, nick_name, icon FROM tb_user WHERE id IN (?)").WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "nick_name", "icon"}).AddRow(9, "作者", nil))
	if got := callSocial(t, router, http.MethodGet, "/follow/common/87", "", true, 200); !strings.Contains(got, `"id":9`) {
		t.Errorf("common follows = %s", got)
	}
	created := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	cols := []string{"id", "shop_id", "user_id", "title", "images", "content", "liked", "comments", "create_time", "update_time"}
	mock.ExpectQuery("SELECT " + blogColumns + ", u.icon, u.nick_name FROM tb_blog b LEFT JOIN tb_user u ON u.id = b.user_id ORDER BY b.liked DESC, b.id DESC LIMIT 10 OFFSET ?").
		WithArgs(int64(0)).WillReturnRows(sqlmock.NewRows(append(append([]string{}, cols...), "icon", "nick_name")).AddRow(11, 3, 9, "探店", "/a.png", "正文", 4, nil, created, created, nil, "作者"))
	if got := callSocial(t, router, http.MethodGet, "/blog/hot", "", false, 200); !strings.Contains(got, `"name":"作者"`) || strings.Contains(got, `"isLike"`) {
		t.Errorf("anonymous hot response = %s", got)
	}
	const stamp int64 = 1795003200000
	for _, id := range []string{"14", "13", "11", "10"} {
		if err := client.ZAdd(ctx, "feed:42", redis.Z{Score: float64(stamp), Member: id}).Err(); err != nil {
			t.Fatal(err)
		}
	}
	if err := client.ZAdd(ctx, "blog:liked:10", redis.Z{Score: float64(stamp), Member: "42"}).Err(); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT " + blogColumns + " FROM tb_blog b WHERE b.id = ?").WithArgs(int64(11)).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(11, 3, 9, "探店", "/a.png", "正文", 0, nil, created, created))
	mock.ExpectQuery("SELECT " + blogColumns + " FROM tb_blog b WHERE b.id = ?").WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(10, 3, 9, "探店", "/b.png", "正文", 1, nil, created, created))
	var page struct {
		Data struct {
			List []struct {
				ID     int64
				IsLike *bool
			}
			MinTime, Offset int64
		}
	}
	if err := json.Unmarshal([]byte(callSocial(t, router, http.MethodGet, "/blog/of/follow?lastId=1795003200000&offset=2", "", true, 200)), &page); err != nil {
		t.Fatal(err)
	}
	if page.Data.MinTime != stamp || page.Data.Offset != 4 || len(page.Data.List) != 2 || page.Data.List[0].ID != 11 || page.Data.List[0].IsLike == nil || *page.Data.List[0].IsLike {
		t.Errorf("unexpected feed page %+v", page.Data)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
