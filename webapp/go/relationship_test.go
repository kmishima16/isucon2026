package isureal

import (
	"context"
	"errors"
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
func TestRelationshipLocksOnlyThePair(t *testing.T) {
	dsn := os.Getenv("ISUREAL_TEST_DSN")
	if dsn == "" {
		t.Skip("ISUREAL_TEST_DSN is not set")
	}
	db := sqlx.MustOpen("pgx", dsn)
	defer db.Close()
	db.MustExec(`TRUNCATE follows, friendships, friend_requests, block_relations, users CASCADE`)
	db.MustExec(`INSERT INTO users (id, username, secret_key_hash, display_name, region, account_type, status) VALUES
	  ('a', 'a', '0000000000000000000000000000000000000000000000000000000000000000', 'A', 'asia', 'regular', 'active'), ('b', 'b', '0000000000000000000000000000000000000000000000000000000000000000', 'B', 'asia', 'regular', 'active'),
	  ('star', 'star', '0000000000000000000000000000000000000000000000000000000000000000', 'Star', 'asia', 'certified_public', 'active'), ('gone', 'gone', '0000000000000000000000000000000000000000000000000000000000000000', 'Gone', 'asia', 'regular', 'deleted')`)
	ctx := context.Background()

	held, err := beginRelationshipTx(ctx, db, "a", "star")
	if err != nil {
		t.Fatal(err)
	}
	// ほかの人から同じ相手への操作は待たない
	quick, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	other, err := beginRelationshipTx(quick, db, "b", "star")
	cancel()
	if err != nil {
		t.Fatalf("another pair waited: %v", err)
	}
	other.Rollback()
	// 同じ 2 人の操作は待つ（逆向きでも）
	quick, cancel = context.WithTimeout(ctx, 500*time.Millisecond)
	same, err := beginRelationshipTx(quick, db, "star", "a")
	cancel()
	if err == nil {
		same.Rollback()
		t.Fatal("same pair did not wait")
	}
	held.Rollback()

	for _, target := range []string{"gone", "missing"} {
		_, err := beginRelationshipTx(ctx, db, "a", target)
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusNotFound {
			t.Fatalf("target %s: %v", target, err)
		}
	}

	h := &HandlerContext{DB: db, Config: &Config{Region: "asia"}}
	e := echo.New()
	e.HTTPErrorHandler = APIErrorHandler
	asUser := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Set("session", &Session{UserID: c.Request().Header.Get("X-Test-User"), Region: "asia"})
			return next(c)
		}
	}
	e.POST("/api/friend-requests", createFriendRequestHandler(h), asUser)
	e.PUT("/api/follows/:userId", followHandler(h), asUser)
	e.PUT("/api/blocks/:userId", blockHandler(h), asUser)
	do := func(user, method, path, body string) int {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-User", user)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec.Code
	}
	steps := []struct {
		user, method, path, body string
		want                     int
	}{
		{"a", http.MethodPost, "/api/friend-requests", `{"recipientId":"b"}`, http.StatusCreated},
		{"a", http.MethodPost, "/api/friend-requests", `{"recipientId":"b"}`, http.StatusConflict},
		{"b", http.MethodPost, "/api/friend-requests", `{"recipientId":"a"}`, http.StatusCreated},
		{"a", http.MethodPost, "/api/friend-requests", `{"recipientId":"b"}`, http.StatusConflict},
		{"a", http.MethodPost, "/api/friend-requests", `{"recipientId":"gone"}`, http.StatusNotFound},
		{"a", http.MethodPut, "/api/follows/star", ``, http.StatusNoContent},
		{"a", http.MethodPut, "/api/follows/star", ``, http.StatusConflict},
		{"a", http.MethodPut, "/api/follows/b", ``, http.StatusConflict},
		{"b", http.MethodPut, "/api/blocks/a", ``, http.StatusNoContent},
		{"a", http.MethodPost, "/api/friend-requests", `{"recipientId":"b"}`, http.StatusConflict},
	}
	for i, step := range steps {
		if got := do(step.user, step.method, step.path, step.body); got != step.want {
			t.Fatalf("step %d %s %s %s: got %d want %d", i, step.user, step.method, step.path, got, step.want)
		}
	}
	var friendships int
	db.Get(&friendships, `SELECT count(*) FROM friendships`)
	if friendships != 0 {
		t.Fatalf("block should remove the friendship, %d left", friendships)
	}
}
