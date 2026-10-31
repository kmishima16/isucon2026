package isureal

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v5"
)

// DB につながらない状態で、SSE の接続と投稿期間の開始の通知が、DB を使わずに済むことを確かめる
func TestNotificationsWithoutDatabaseRoundTrips(t *testing.T) {
	// 接続できない DB。使えばすぐ失敗する
	db := sqlx.MustOpen("pgx", "postgres://x:x@127.0.0.1:1/x?connect_timeout=1")
	defer db.Close()
	h := &HandlerContext{DB: db, Config: &Config{Region: "europe"}, Windows: newPostingWindowHub()}
	registeredAt := time.Now().Add(-time.Hour)
	e := echo.New()
	e.HTTPErrorHandler = APIErrorHandler
	e.GET("/api/notifications", getNotificationsHandler(h), func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Set("session", &Session{UserID: c.Request().URL.Query().Get("user"), Region: "europe", UserCreatedAt: registeredAt})
			return next(c)
		}
	})
	server := httptest.NewServer(e)
	defer server.Close()

	type result struct {
		status int
		body   string
	}
	subscribe := func(userID string) chan result {
		ch := make(chan result, 1)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/notifications?user="+userID, nil)
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				ch <- result{}
				return
			}
			defer res.Body.Close()
			line, _ := bufio.NewReader(res.Body).ReadString('\n')
			ch <- result{status: res.StatusCode, body: strings.TrimSpace(line)}
		}()
		return ch
	}

	// 開始の前につないだ人には、DB に聞かずに届く
	waiting := subscribe("u1")
	time.Sleep(200 * time.Millisecond)
	h.Windows.set(&activeWindow{ID: "w1", Region: "europe", StartsAt: time.Now()})
	if got := <-waiting; got.status != http.StatusOK || !strings.Contains(got.body, `"postingWindowId":"w1"`) {
		t.Fatalf("notification without DB: %+v", got)
	}

	// 接続の後にこの台で利用者が変わったら、DB で確かめ直す。ここでは DB が使えないので、通知せずに閉じる
	changed := subscribe("u2")
	time.Sleep(200 * time.Millisecond)
	h.Windows.UserChanged()
	if got := <-changed; got.status != http.StatusOK || got.body != "" {
		t.Fatalf("user changed without DB: %+v", got)
	}
}
