package isureal

import (
	"crypto/sha256"
	"encoding/hex"
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
func TestLoginAndRegisterInOneRoundTrip(t *testing.T) {
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
	db.MustExec(`TRUNCATE login_sessions, users CASCADE`)
	hashOf := func(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) }
	db.MustExec(`INSERT INTO users (id, username, secret_key_hash, display_name, region) VALUES ('u1', 'alice', $1, 'Alice', 'europe'), ('gone', 'gone', $1, 'G', 'asia')`, hashOf("secret"))
	db.MustExec(`UPDATE users SET status = 'deleted' WHERE id = 'gone'`)

	h := &HandlerContext{DB: db, Config: &Config{Region: "europe"}}
	e := echo.New()
	e.HTTPErrorHandler = APIErrorHandler
	e.POST("/api/auth/login", loginHandler(h))
	e.POST("/api/auth/register", registerHandler(h))
	call := func(path, body string) (int, map[string]any) {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		var res map[string]any
		json.Unmarshal(rec.Body.Bytes(), &res)
		return rec.Code, res
	}
	login := func(username, secret string) (int, map[string]any) {
		return call("/api/auth/login", `{"username":"`+username+`","secretKey":"`+secret+`"}`)
	}

	login("alice", "secret")
	before := proxy.writes.Load()
	code, res := login("alice", "secret")
	t.Logf("round trips for a login: %d", proxy.writes.Load()-before)
	if code != http.StatusOK || res["token"] == "" {
		t.Fatalf("login: %d %v", code, res)
	}
	if trips := proxy.writes.Load() - before; trips > 1 {
		t.Fatalf("login took %d round trips", trips)
	}
	user, _ := res["user"].(map[string]any)
	if user["id"] != "u1" || user["username"] != "alice" || user["region"] != "europe" || user["status"] != "active" {
		t.Fatalf("login user: %v", user)
	}
	var active int
	db.Get(&active, `SELECT count(*) FROM login_sessions WHERE user_id = 'u1' AND active AND token_hash = $1`, hashOf(res["token"].(string)))
	if active != 1 {
		t.Fatal("the new token is not an active session")
	}
	db.Get(&active, `SELECT count(*) FROM login_sessions WHERE user_id = 'u1' AND active`)
	if active != 1 {
		t.Fatalf("older sessions are still active: %d", active)
	}
	for _, c := range []struct{ username, secret string }{{"alice", "wrong"}, {"gone", "secret"}, {"nobody", "secret"}} {
		if code, res := login(c.username, c.secret); code != http.StatusUnauthorized || res["error"] != "invalid_credentials" {
			t.Fatalf("login %s: %d %v", c.username, code, res)
		}
	}
	var sessions int
	db.Get(&sessions, `SELECT count(*) FROM login_sessions`)
	if sessions != 2 {
		t.Fatalf("failed logins wrote sessions: %d rows", sessions)
	}

	before = proxy.writes.Load()
	code, res = call("/api/auth/register", `{"username":"bob","displayName":"Bob","region":"america"}`)
	t.Logf("round trips for a register: %d", proxy.writes.Load()-before)
	user, _ = res["user"].(map[string]any)
	if code != http.StatusCreated || res["secretKey"] == "" || user["username"] != "bob" || user["region"] != "america" || user["accountType"] != "regular" || user["status"] != "active" {
		t.Fatalf("register: %d %v", code, res)
	}
	if code, res := call("/api/auth/register", `{"username":"bob","displayName":"Bob","region":"asia"}`); code != http.StatusConflict || res["error"] != "username_taken" {
		t.Fatalf("duplicate register: %d %v", code, res)
	}
	if code, _ := login("bob", res["secretKey"].(string)); code != http.StatusOK {
		t.Fatalf("login with the issued key: %d", code)
	}
}
