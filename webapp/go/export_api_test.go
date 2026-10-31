package isureal

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
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
func TestExportUsesImageDataCache(t *testing.T) {
	dsn := os.Getenv("ISUREAL_TEST_DSN")
	if dsn == "" {
		t.Skip("ISUREAL_TEST_DSN is not set")
	}
	db := sqlx.MustOpen("pgx", dsn)
	defer db.Close()
	db.MustExec(`TRUNCATE realmojis, comments, posts, posting_windows, users, image_objects CASCADE`)
	db.MustExec(`INSERT INTO users (id, username, secret_key_hash, display_name, region) VALUES ('u1', 'u1', $1, 'U1', 'asia')`, strings.Repeat("0", 64))
	db.MustExec(`INSERT INTO posting_windows (id, region, local_date, starts_at, on_time_deadline, ended_at) VALUES
	  ('w1', 'asia', '2025-03-01', '2025-03-01 00:00:00+00', '2025-03-01 00:02:00+00', '2025-03-02 00:00:00+00'),
	  ('w2', 'asia', '2025-03-02', '2025-03-02 00:00:00+00', '2025-03-02 00:02:00+00', NULL)`)
	db.MustExec(`INSERT INTO image_objects (id, owner_user_id, data, media_type, created_at) VALUES
	  ('i1', 'u1', 'ONE', 'image/jpeg', '2025-03-01 00:01:00+00'),
	  ('i2', 'u1', 'TWO', 'image/jpeg', '2025-03-02 00:05:00+00'),
	  ('i3', 'u1', 'GONE', 'image/jpeg', '2025-03-02 00:06:00+00')`)
	db.MustExec(`INSERT INTO posts (id, author_id, posting_window_id, image_id, visibility, location_kind, posted_at, deleted_at) VALUES
	  ('p1', 'u1', 'w1', 'i1', 'public', 'none', '2025-03-01 00:01:00+00', NULL),
	  ('p2', 'u1', 'w2', 'i2', 'friends', 'none', '2025-03-02 00:05:00+00', NULL),
	  ('p3', 'u1', 'w2', 'i3', 'friends', 'none', '2025-03-02 00:06:00+00', now())`)

	h := &HandlerContext{DB: db, Config: &Config{Region: "asia"}, ImageData: newImageDataCache()}
	e := echo.New()
	e.HTTPErrorHandler = APIErrorHandler
	e.GET("/api/me/export", exportPostsHandler(h), func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Set("session", &Session{UserID: "u1", Region: "asia"})
			return next(c)
		}
	})
	export := func() map[string]string {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/me/export", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("export: %d %s", rec.Code, rec.Body.String())
		}
		r, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
		if err != nil {
			t.Fatal(err)
		}
		files := map[string]string{}
		for _, f := range r.File {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			files[f.Name] = string(b)
		}
		return files
	}

	cold := export()
	if cold["images/000001.jpg"] != "ONE" || cold["images/000002.jpg"] != "TWO" || len(cold) != 3 {
		t.Fatalf("cold export: %v", cold)
	}
	// 2 回目は台のメモリから返す（DB の中身を変えても前のまま）
	db.MustExec(`UPDATE image_objects SET data = 'CHANGED'`)
	warm := export()
	for name, body := range cold {
		if warm[name] != body {
			t.Fatalf("warm export differs at %s: %q vs %q", name, warm[name], body)
		}
	}
	// 一部だけ持っているときも、持っていない分は DB から読む
	h.ImageData = newImageDataCache()
	h.ImageData.set("i1", []byte("ONE"))
	mixed := export()
	if mixed["images/000001.jpg"] != "ONE" || mixed["images/000002.jpg"] != "CHANGED" {
		t.Fatalf("mixed export: %v", mixed)
	}
}

// ISUREAL_TEST_DSN に空の DB（001-schema.sql と sql/*.sql を流したもの）を渡したときだけ動く
func TestExportPrefetchesSignatures(t *testing.T) {
	dsn := os.Getenv("ISUREAL_TEST_DSN")
	if dsn == "" {
		t.Skip("ISUREAL_TEST_DSN is not set")
	}
	db := sqlx.MustOpen("pgx", dsn)
	defer db.Close()
	db.MustExec(`TRUNCATE realmojis, comments, posts, posting_windows, users, image_objects CASCADE`)
	db.MustExec(`INSERT INTO users (id, username, secret_key_hash, display_name, profile_image_id, region) VALUES ('u1', 'u1', $1, 'U1', 'pimg-u1', 'asia')`, strings.Repeat("1", 64))
	db.MustExec(`INSERT INTO posting_windows (id, region, local_date, starts_at, on_time_deadline) VALUES ('w1', 'asia', '2025-03-01', '2025-03-01 00:00:00+00', '2025-03-01 00:02:00+00')`)
	for i := 1; i <= 12; i++ {
		at := fmt.Sprintf("2025-03-01 00:%02d:00+00", i)
		db.MustExec(`INSERT INTO image_objects (id, owner_user_id, data, media_type, created_at) VALUES ($1, 'u1', $2, 'image/jpeg', $3)`, fmt.Sprintf("i%02d", i), fmt.Sprintf("DATA%02d", i), at)
		db.MustExec(`INSERT INTO posts (id, author_id, posting_window_id, image_id, visibility, location_kind, posted_at) VALUES ($1, 'u1', 'w1', $2, 'public', 'none', $3)`, fmt.Sprintf("p%02d", i), fmt.Sprintf("i%02d", i), at)
	}
	h := &HandlerContext{DB: db, Config: &Config{Region: "asia"}, ImageData: newImageDataCache()}
	e := echo.New()
	e.HTTPErrorHandler = APIErrorHandler
	e.GET("/api/me/export", exportPostsHandler(h), func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Set("session", &Session{UserID: "u1", Region: "asia"})
			return next(c)
		}
	})
	export := func() (string, int64) {
		signedURLCache = &signatureCache{}
		before := signatureComputations.Load()
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/me/export", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("export: %d %s", rec.Code, rec.Body.String())
		}
		return rec.Body.String(), signatureComputations.Load() - before
	}
	hour := imageURLExp(time.Now())
	imageSigner = nil
	local, localCount := export()
	if localCount == 0 {
		t.Fatal("expected local signing without a remote signer")
	}
	srv := httptest.NewServer(NewSignerHandler())
	defer srv.Close()
	imageSigner = &remoteSigner{peers: []string{srv.URL}, client: srv.Client(), timeout: 5 * time.Second, minBatch: 1}
	defer func() { imageSigner = nil }()
	remote, remoteCount := export()
	if imageURLExp(time.Now()) != hour {
		t.Skip("crossed an hour boundary; URLs differ by design")
	}
	if remoteCount != 0 {
		t.Fatalf("signed %d images locally after prefetch", remoteCount)
	}
	if local != remote {
		t.Fatal("export differs with and without the remote signer")
	}
}
