package isureal

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v5"
)

// ISUREAL_TEST_DSN に空の DB（001-schema.sql を流したもの）を渡したときだけ動く
func TestNotificationsFromPostingWindowHub(t *testing.T) {
	dsn := os.Getenv("ISUREAL_TEST_DSN")
	if dsn == "" {
		t.Skip("ISUREAL_TEST_DSN is not set")
	}
	db := sqlx.MustOpen("pgx", dsn)
	defer db.Close()
	db.MustExec(`TRUNCATE notifications, posts, posting_windows, users CASCADE`)
	db.MustExec(`INSERT INTO users (id, username, secret_key_hash, display_name, region) VALUES
	  ('old', 'old', 'x', 'Old', 'asia'), ('gone', 'gone', 'x', 'Gone', 'asia')`)

	h := &HandlerContext{DB: db, Config: &Config{Region: "asia"}, Windows: newPostingWindowHub()}
	go h.Windows.poll(db, "asia")
	e := echo.New()
	e.HTTPErrorHandler = APIErrorHandler
	e.GET("/api/notifications", getNotificationsHandler(h), func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Set("session", &Session{UserID: c.Request().URL.Query().Get("user"), Region: "asia"})
			return next(c)
		}
	})
	e.POST("/api/admin/posting-window", openPostingWindowHandler(h))
	e.POST("/api/me/account-deletion", deleteAccountHandlerForTest(h))
	server := httptest.NewServer(e)
	defer server.Close()

	type result struct {
		body   string
		closed bool
	}
	subscribe := func(userID string, wait time.Duration) chan result {
		ch := make(chan result, 1)
		ctx, cancel := context.WithTimeout(context.Background(), wait)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/notifications?user="+userID, nil)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			defer cancel()
			defer res.Body.Close()
			line, err := bufio.NewReader(res.Body).ReadString('\n')
			ch <- result{body: strings.TrimSpace(line), closed: err != nil && ctx.Err() == nil}
		}()
		return ch
	}
	openWindow := func(date string) openPostingWindowResponse {
		req := httptest.NewRequest(http.MethodPost, "/api/admin/posting-window", strings.NewReader(`{"region":"asia","localDate":"`+date+`","onTimeSeconds":10}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("open window: %d %s", rec.Code, rec.Body.String())
		}
		var window openPostingWindowResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &window)
		return window
	}

	// 開始の前につないだ利用者には、開始してすぐ届く
	first := subscribe("old", 3*time.Second)
	time.Sleep(200 * time.Millisecond)
	window := openWindow("2026-11-01")
	got := <-first
	var notification getNotificationsResponse
	if err := json.Unmarshal([]byte(strings.TrimPrefix(got.body, "data: ")), &notification); err != nil {
		t.Fatalf("notification: %q %v", got.body, err)
	}
	if notification.PostingWindowID != window.ID || notification.Region != "asia" || notification.ID != notificationID("old", window.ID) || !notification.CreatedAt.Equal(window.StartsAt) {
		t.Fatalf("unexpected notification %+v for window %+v", notification, window)
	}

	// 開始の後につないだ利用者には、その投稿期間の通知は届かない
	late := subscribe("old", 500*time.Millisecond)
	if got := <-late; got.body != "" {
		t.Fatalf("late subscriber got %q", got.body)
	}

	// 開始の後に登録した利用者には、その投稿期間の通知は届かない。次の投稿期間では届く
	waiting := subscribe("old", 3*time.Second)
	time.Sleep(200 * time.Millisecond)
	db.MustExec(`INSERT INTO users (id, username, secret_key_hash, display_name, region) VALUES ('new', 'new', 'x', 'New', 'asia')`)
	newcomer := subscribe("new", 3*time.Second)
	time.Sleep(200 * time.Millisecond)
	// ほかの台が開いた投稿期間は、見張りの問い合わせで拾う
	db.MustExec(`UPDATE posting_windows SET ended_at = clock_timestamp() WHERE region = 'asia' AND ended_at IS NULL`)
	db.MustExec(`INSERT INTO posting_windows (id, region, local_date, starts_at, on_time_deadline) VALUES ('other-server', 'asia', '2026-11-02', clock_timestamp(), clock_timestamp() + interval '10 seconds')`)
	if got := <-waiting; !strings.Contains(got.body, `"postingWindowId":"other-server"`) {
		t.Fatalf("window opened elsewhere: %q", got.body)
	}
	if got := <-newcomer; !strings.Contains(got.body, `"postingWindowId":"other-server"`) {
		t.Fatalf("newcomer for next window: %q", got.body)
	}
	db.MustExec(`UPDATE users SET created_at = clock_timestamp() + interval '1 minute' WHERE id = 'new'`)
	afterStart := subscribe("new", 3*time.Second)
	time.Sleep(200 * time.Millisecond)
	db.MustExec(`UPDATE posting_windows SET ended_at = clock_timestamp() WHERE region = 'asia' AND ended_at IS NULL`)
	db.MustExec(`INSERT INTO posting_windows (id, region, local_date, starts_at, on_time_deadline) VALUES ('w3', 'asia', '2026-11-03', clock_timestamp(), clock_timestamp() + interval '10 seconds')`)
	if got := <-afterStart; got.body != "" {
		t.Fatalf("user registered after window start got %q", got.body)
	}

	// この台で退会したら、待っている接続は閉じる
	gone := subscribe("gone", 3*time.Second)
	time.Sleep(200 * time.Millisecond)
	db.MustExec(`UPDATE users SET status = 'deleted' WHERE id = 'gone'`)
	h.Windows.UserChanged("gone")
	if got := <-gone; got.body != "" || !got.closed {
		t.Fatalf("deleted user stream: %+v", got)
	}
}

func deleteAccountHandlerForTest(h *HandlerContext) echo.HandlerFunc {
	return func(c *echo.Context) error {
		h.Windows.UserChanged(c.QueryParam("user"))
		return c.NoContent(http.StatusNoContent)
	}
}

func TestNotificationIDIsStable(t *testing.T) {
	if notificationID("u", "w") != notificationID("u", "w") || notificationID("u", "w") == notificationID("u", "w2") {
		t.Fatal("notification id must depend only on user and window")
	}
}
