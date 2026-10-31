package isureal

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
)

func TestIsLowPriority(t *testing.T) {
	cases := []struct {
		method, path string
		want         bool
	}{
		{"GET", "/api/images/abc", true},
		{"GET", "/api/memories", true},
		{"GET", "/api/memories/2026-10-31", true},
		{"GET", "/api/me/export", true},
		{"GET", "/api/users", true},
		{"GET", "/api/users/u1", true},
		{"GET", "/api/feeds/public", true},
		{"GET", "/api/feeds/following", true},
		{"GET", "/api/feeds/friends-of-friends", true},
		{"GET", "/api/posts/p1", true},
		{"GET", "/api/posts/p1/comments", true},
		{"GET", "/api/relationships/friends", true},
		// 新しい人の経路と、友達フィード・通知・書き込みは絞らない
		{"GET", "/api/feeds/friends", false},
		{"GET", "/api/posts/me/current", false},
		{"GET", "/api/posting-window/current", false},
		{"GET", "/api/notifications", false},
		{"GET", "/api/me", false},
		{"POST", "/api/posts", false},
		{"POST", "/api/posts/p1/comments", false},
		{"POST", "/api/friend-requests", false},
		{"POST", "/api/auth/login", false},
		{"POST", "/api/initialize", false},
		{"POST", "/api/admin/posting-window", false},
	}
	for _, tc := range cases {
		if got := isLowPriority(tc.method, tc.path); got != tc.want {
			t.Errorf("isLowPriority(%s %s) = %v, want %v", tc.method, tc.path, got, tc.want)
		}
	}
}

// 枠が埋まっていても、上限の時間を過ぎたら絞らずに通す（走行後の確認を時間切れにしない）
func TestLowPriorityLimiterPassesAfterWait(t *testing.T) {
	l := newLowPriorityLimiter(1, 50*time.Millisecond)
	l.slots <- struct{}{} // 枠を埋めておく
	e := echo.New()
	var called atomic.Int32
	h := LowPriorityLimitMiddleware(l)(func(c *echo.Context) error {
		called.Add(1)
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/memories", nil)
	rec := httptest.NewRecorder()
	start := time.Now()
	if err := h(e.NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond || elapsed > time.Second {
		t.Fatalf("waited %v, want about 50ms", elapsed)
	}
	if called.Load() != 1 {
		t.Fatal("handler was not called after the wait")
	}
	if len(l.slots) != 1 {
		t.Fatalf("slots in use = %d, want 1 (the request passed without taking a slot)", len(l.slots))
	}

	// 絞らない経路は、枠が埋まっていても待たない
	req = httptest.NewRequest(http.MethodPost, "/api/posts", nil)
	start = time.Now()
	if err := h(e.NewContext(req, httptest.NewRecorder())); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 20*time.Millisecond {
		t.Fatalf("priority request waited %v", elapsed)
	}
}

// 枠が空いていれば待たずに通り、終わったら枠を返す
func TestLowPriorityLimiterReleasesSlot(t *testing.T) {
	l := newLowPriorityLimiter(1, time.Second)
	e := echo.New()
	h := LowPriorityLimitMiddleware(l)(func(c *echo.Context) error {
		if len(l.slots) != 1 {
			t.Errorf("slots in use during handler = %d, want 1", len(l.slots))
		}
		return c.NoContent(http.StatusOK)
	})
	for range 3 {
		req := httptest.NewRequest(http.MethodGet, "/api/images/x", nil)
		if err := h(e.NewContext(req, httptest.NewRecorder())); err != nil {
			t.Fatal(err)
		}
	}
	if len(l.slots) != 0 {
		t.Fatalf("slots in use after = %d, want 0", len(l.slots))
	}
}

// 絞りを入れない台（未設定）では、何もしない
func TestLowPriorityLimiterDisabledByDefault(t *testing.T) {
	t.Setenv("ISUREAL_LOW_PRIORITY_SLOTS", "")
	if l := newLowPriorityLimiterFromEnv(); l != nil {
		t.Fatal("limiter should be nil when ISUREAL_LOW_PRIORITY_SLOTS is not set")
	}
}
