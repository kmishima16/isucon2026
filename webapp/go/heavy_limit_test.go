package isureal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
)

func TestHeavyLimitRunsOneAtATime(t *testing.T) {
	e := echo.New()
	heavy := HeavyLimitMiddleware(1)
	var running, peak, done atomic.Int32
	slow := func(c *echo.Context) error {
		now := running.Add(1)
		for {
			p := peak.Load()
			if now <= p || peak.CompareAndSwap(p, now) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		running.Add(-1)
		done.Add(1)
		return c.NoContent(http.StatusOK)
	}
	e.GET("/api/memories", slow, heavy)
	e.GET("/api/me/export", slow, heavy)
	e.GET("/api/feeds/friends", func(c *echo.Context) error { return c.NoContent(http.StatusOK) })
	release := make(chan struct{})
	e.GET("/api/memories/:date", func(c *echo.Context) error { <-release; return c.NoContent(http.StatusOK) }, heavy)

	var wg sync.WaitGroup
	start := time.Now()
	for i := range 4 {
		path := "/api/memories"
		if i%2 == 1 {
			path = "/api/me/export"
		}
		wg.Go(func() {
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		})
	}
	// 絞っていない API は待たない
	time.Sleep(10 * time.Millisecond)
	quick := time.Now()
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/feeds/friends", nil))
	if rec.Code != http.StatusOK || time.Since(quick) > 20*time.Millisecond {
		t.Fatalf("unlimited route waited %v", time.Since(quick))
	}
	wg.Wait()
	if peak.Load() != 1 || done.Load() != 4 {
		t.Fatalf("peak concurrency %d, done %d", peak.Load(), done.Load())
	}
	if elapsed := time.Since(start); elapsed < 200*time.Millisecond {
		t.Fatalf("4 heavy requests finished in %v, expected them to run one at a time", elapsed)
	}

	// 待っている間に接続が切れたら、処理を走らせずに抜ける
	go e.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/memories/2026-10-31", nil))
	time.Sleep(10 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	waited := make(chan struct{})
	before := done.Load()
	go func() {
		e.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/memories", nil).WithContext(ctx))
		close(waited)
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("cancelled request kept waiting")
	}
	close(release)
	time.Sleep(20 * time.Millisecond)
	if done.Load() != before {
		t.Fatal("cancelled request ran the handler")
	}
	// 枠は返っている
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/memories", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("slot was not released: %d", rec.Code)
	}
}
