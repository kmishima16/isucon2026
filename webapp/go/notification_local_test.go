package isureal

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v5"
)

// DB につながらない状態で、SSE が DB を使わずに通知し、その利用者の変更を知らされたときだけ DB で確かめ直すことを確かめる
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

	// ほかの利用者の変更では確かめ直さず、開始のときに DB に聞かずに届く
	waiting := subscribe("u1")
	time.Sleep(200 * time.Millisecond)
	h.Windows.userChangedLocal("someone-else")
	time.Sleep(100 * time.Millisecond)
	h.Windows.set(&activeWindow{ID: "w1", Region: "europe", StartsAt: time.Now()})
	if got := <-waiting; got.status != http.StatusOK || !strings.Contains(got.body, `"postingWindowId":"w1"`) {
		t.Fatalf("notification without DB: %+v", got)
	}

	// その利用者の変更（退会・リージョンの移動）を知らされたら、DB で確かめ直す。ここでは DB が使えないので、通知せずに閉じる
	changed := subscribe("u2")
	time.Sleep(200 * time.Millisecond)
	h.Windows.userChangedLocal("u2")
	if got := <-changed; got.status != http.StatusOK || got.body != "" {
		t.Fatalf("changed user must be checked against DB: %+v", got)
	}

	// 初期化（全員の確かめ直し）でも、DB で確かめ直す
	reset := subscribe("u3")
	time.Sleep(200 * time.Millisecond)
	h.Windows.userChangedLocal("")
	if got := <-reset; got.status != http.StatusOK || got.body != "" {
		t.Fatalf("all users must be checked against DB after reset: %+v", got)
	}
}

// 退会とリージョンの変更は、自分以外の全台に利用者を付けて知らせ、届くのを待ってから戻る
func TestNotifyUserChangedWaitsForAllPeers(t *testing.T) {
	var mu sync.Mutex
	got := map[string]string{}
	newPeer := func(name string) *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			mu.Lock()
			got[name] = r.URL.Path + "?" + r.URL.RawQuery
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(srv.Close)
		return srv
	}
	saved := images.peers
	images.peers = []string{newPeer("a").URL, newPeer("b").URL, newPeer("c").URL}
	t.Cleanup(func() { images.peers = saved })

	images.notifyUserChanged("u/1")
	mu.Lock()
	defer mu.Unlock()
	for _, name := range []string{"a", "b", "c"} {
		if got[name] != "/internal/user-changed?user=u%2F1" {
			t.Fatalf("peer %s got %q before notifyUserChanged returned", name, got[name])
		}
	}
}
