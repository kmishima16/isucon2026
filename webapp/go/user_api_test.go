package isureal

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v5"
)

// ISUREAL_TEST_DSN に空の DB（001-schema.sql と sql/002_users_username.sql を流したもの）を渡したときだけ動く
func TestRegisterUsernameUnique(t *testing.T) {
	dsn := os.Getenv("ISUREAL_TEST_DSN")
	if dsn == "" {
		t.Skip("ISUREAL_TEST_DSN is not set")
	}
	db := sqlx.MustOpen("pgx", dsn)
	defer db.Close()
	db.MustExec(`TRUNCATE users CASCADE`)
	db.MustExec(`INSERT INTO users (id, username, secret_key_hash, display_name, region, status) VALUES ('gone', 'gone', 'x', 'Gone', 'asia', 'deleted')`)

	h := &HandlerContext{DB: db, Config: &Config{}}
	e := echo.New()
	e.HTTPErrorHandler = APIErrorHandler
	e.POST("/api/auth/register", registerHandler(h))
	register := func(username string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"`+username+`","displayName":"D","region":"europe"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := register("alice"); code != http.StatusCreated {
		t.Fatalf("first register: %d", code)
	}
	if code := register("alice"); code != http.StatusConflict {
		t.Fatalf("duplicate register: %d", code)
	}
	if code := register("gone"); code != http.StatusConflict {
		t.Fatalf("register with deleted user's name: %d", code)
	}

	codes := make(chan int, 8)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() { codes <- register("bob") })
	}
	wg.Wait()
	close(codes)
	created := 0
	for code := range codes {
		switch code {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
		default:
			t.Fatalf("concurrent register: %d", code)
		}
	}
	if created != 1 {
		t.Fatalf("concurrent register created %d users", created)
	}
}
