package isureal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v5"
)

// ISUREAL_TEST_DSN に空の DB（001-schema.sql と sql/ を流したもの）を渡したときだけ動く
func TestCreateFriendRequestInOneRoundTrip(t *testing.T) {
	dsn := os.Getenv("ISUREAL_TEST_DSN")
	if dsn == "" {
		t.Skip("ISUREAL_TEST_DSN is not set")
	}
	u, _ := url.Parse(dsn)
	proxy := startDelayProxy(t, u.Host)
	u.Host = proxy.addr
	db := sqlx.MustOpen("pgx", u.String())
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.MustExec(`TRUNCATE follows, friendships, friend_requests, block_relations, users CASCADE`)
	key := strings.Repeat("0", 64)
	db.MustExec(`INSERT INTO users (id, username, secret_key_hash, display_name, region) VALUES
	  ('a', 'a', $1, 'A', 'asia'), ('b', 'b', $1, 'B', 'asia'), ('c', 'c', $1, 'C', 'asia'), ('d', 'd', $1, 'D', 'asia'), ('gone', 'gone', $1, 'G', 'asia')`, key)
	db.MustExec(`UPDATE users SET status = 'deleted' WHERE id = 'gone'`)
	db.MustExec(`INSERT INTO block_relations (id, blocker_id, blocked_id) VALUES ('bl', 'd', 'a')`)

	h := &HandlerContext{DB: db, Config: &Config{Region: "asia"}}
	e := echo.New()
	e.HTTPErrorHandler = APIErrorHandler
	e.POST("/api/friend-requests", createFriendRequestHandler(h), func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Set("session", &Session{UserID: c.Request().Header.Get("X-Test-User"), Region: "asia"})
			return next(c)
		}
	})
	send := func(actor, recipient string) (int, map[string]any) {
		req := httptest.NewRequest(http.MethodPost, "/api/friend-requests", strings.NewReader(`{"recipientId":"`+recipient+`"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-User", actor)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		var res map[string]any
		json.Unmarshal(rec.Body.Bytes(), &res)
		return rec.Code, res
	}

	send("c", "d")
	before := proxy.writes.Load()
	code, res := send("a", "b")
	trips := proxy.writes.Load() - before
	t.Logf("round trips for a friend request: %d", trips)
	if code != http.StatusCreated || res["status"] != "pending" || res["relationId"] == "" || res["updatedAt"] == nil {
		t.Fatalf("new request: %d %v", code, res)
	}
	if trips > 1 {
		t.Fatalf("friend request took %d round trips", trips)
	}
	first := res["relationId"]
	steps := []struct {
		actor, recipient string
		want             int
		error            string
	}{
		{"a", "a", http.StatusBadRequest, "invalid_relationship"},
		{"a", "", http.StatusBadRequest, "invalid_relationship"},
		{"a", "missing", http.StatusNotFound, "relationship_not_found"},
		{"a", "gone", http.StatusNotFound, "relationship_not_found"},
		{"a", "d", http.StatusConflict, "relationship_conflict"},
		{"a", "b", http.StatusConflict, "relationship_conflict"},
	}
	for i, s := range steps {
		if code, res := send(s.actor, s.recipient); code != s.want || res["error"] != s.error {
			t.Fatalf("step %d %s→%s: %d %v", i, s.actor, s.recipient, code, res)
		}
	}
	// 逆向きの申請は、相手の申請を承認して友達になる
	if code, res := send("b", "a"); code != http.StatusCreated || res["status"] != "accepted" || res["relationId"] != first {
		t.Fatalf("reverse request: %d %v", code, res)
	}
	var friendships int
	db.Get(&friendships, `SELECT count(*) FROM friendships WHERE user_low_id = 'a' AND user_high_id = 'b' AND source_request_id = $1`, first)
	if friendships != 1 {
		t.Fatalf("friendship rows: %d", friendships)
	}
	if code, res := send("a", "b"); code != http.StatusConflict || res["error"] != "relationship_conflict" {
		t.Fatalf("request to a friend: %d %v", code, res)
	}
}
