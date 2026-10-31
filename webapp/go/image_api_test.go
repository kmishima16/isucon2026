package isureal

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v5"
)

// ISUREAL_TEST_DSN に空の DB（001-schema.sql を流したもの）を渡したときだけ動く
func TestGetImageWithDataCache(t *testing.T) {
	dsn := os.Getenv("ISUREAL_TEST_DSN")
	if dsn == "" {
		t.Skip("ISUREAL_TEST_DSN is not set")
	}
	db := sqlx.MustOpen("pgx", dsn)
	defer db.Close()
	hashOf := func(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) }
	db.MustExec(`TRUNCATE realmojis, posts, posting_windows, users, image_objects CASCADE`)
	db.MustExec(`INSERT INTO users (id, username, secret_key_hash, display_name, region, profile_image_id) VALUES
	  ('u1', 'u1', $1, 'U1', 'asia', 'img-profile'), ('u2', 'u2', $2, 'U2', 'asia', NULL)`, hashOf("k1"), hashOf("k2"))
	db.MustExec(`INSERT INTO image_objects (id, owner_user_id, data, media_type) VALUES
	  ('img-post', 'u1', 'POST', 'image/jpeg'), ('img-profile', 'u1', 'PROFILE', 'image/jpeg'),
	  ('img-moji', 'u2', 'MOJI', 'image/jpeg'), ('img-orphan', 'u1', 'ORPHAN', 'image/jpeg')`)
	db.MustExec(`INSERT INTO posting_windows (id, region, local_date, starts_at, on_time_deadline) VALUES ('w1', 'asia', '2026-10-31', now(), now() + interval '1 day')`)
	db.MustExec(`INSERT INTO posts (id, author_id, posting_window_id, image_id, visibility, location_kind, posted_at) VALUES ('p1', 'u1', 'w1', 'img-post', 'public', 'none', now())`)
	db.MustExec(`INSERT INTO realmojis (id, post_id, reactor_id, image_id, reaction_type, created_at) VALUES ('r1', 'p1', 'u2', 'img-moji', 'like', now())`)

	h := &HandlerContext{DB: db, Config: &Config{}, ImageData: newImageDataCache(), ImageOwners: newImageOwnerKeyCache(100)}
	e := echo.New()
	e.HTTPErrorHandler = APIErrorHandler
	var abuse string
	e.GET("/api/images/:imageId", getImageHandler(h), func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			err := next(c)
			abuse, _ = c.Get("abuse").(string)
			return err
		}
	})
	keys := map[string][]byte{}
	for user, secret := range map[string]string{"u1": "k1", "u2": "k2"} {
		keys[user], _ = imageSigningKey(hashOf(secret))
	}
	get := func(owner, imageID string, tamper bool) (int, string) {
		exp := time.Now().Add(time.Hour).Unix()
		sig := imageSignature(keys[owner], imageID, exp)
		if tamper {
			sig[0] ^= 1
		}
		req := httptest.NewRequest(http.MethodGet, "/api/images/"+imageID+"?exp="+strconv.FormatInt(exp, 10)+"&sig="+base64.RawURLEncoding.EncodeToString(sig), nil)
		rec := httptest.NewRecorder()
		abuse = ""
		e.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	expect := func(name string, gotCode int, gotBody string, wantCode int, wantBody string, wantAbuse string) {
		t.Helper()
		if gotCode != wantCode || (wantBody != "" && gotBody != wantBody) || abuse != wantAbuse {
			t.Fatalf("%s: got %d %q abuse=%q, want %d %q abuse=%q", name, gotCode, gotBody, abuse, wantCode, wantBody, wantAbuse)
		}
	}

	code, body := get("u1", "img-post", false)
	expect("post image", code, body, 200, "POST", "")
	// 持ち主の鍵は 2 回目から台のメモリで確かめる。退会や削除は、下の生死確認で毎回 DB に聞く
	if _, ok := h.ImageOwners.get("img-post"); !ok {
		t.Fatal("owner key was not cached")
	}
	// 中身は 2 回目から台のメモリから返す（DB の中身を変えても前のまま）
	db.MustExec(`UPDATE image_objects SET data = 'CHANGED' WHERE id = 'img-post'`)
	code, body = get("u1", "img-post", false)
	expect("post image from cache", code, body, 200, "POST", "")
	code, body = get("u1", "img-profile", false)
	expect("profile image", code, body, 200, "PROFILE", "")
	code, body = get("u2", "img-moji", false)
	expect("realmoji image", code, body, 200, "MOJI", "")
	code, body = get("u1", "img-orphan", false)
	expect("unreferenced image", code, body, 403, "", "")
	code, body = get("u1", "img-post", true)
	expect("tampered signature", code, body, 403, "", "invalid_signature")
	code, body = get("u1", "img-missing", false)
	expect("missing image", code, body, 403, "", "")

	// 生きているかは毎回 DB に聞くので、ほかの台での削除もすぐ効く
	db.MustExec(`UPDATE posts SET deleted_at = now() WHERE id = 'p1'`)
	code, body = get("u1", "img-post", false)
	expect("deleted post", code, body, 403, "", "")
	code, body = get("u2", "img-moji", false)
	expect("realmoji on deleted post", code, body, 403, "", "")
	db.MustExec(`UPDATE users SET profile_image_id = NULL WHERE id = 'u1'`)
	code, body = get("u1", "img-profile", false)
	expect("replaced profile image", code, body, 403, "", "")
	db.MustExec(`UPDATE users SET profile_image_id = 'img-profile', status = 'deleted' WHERE id = 'u1'`)
	code, body = get("u1", "img-profile", false)
	expect("deleted owner", code, body, 403, "", "")
	code, body = get("u1", "img-profile", true)
	expect("deleted owner with tampered signature", code, body, 403, "", "")
}

func TestImageDataCacheEviction(t *testing.T) {
	c := newImageDataCache()
	chunk := make([]byte, imageDataCacheBytes/16)
	for i := range 20 {
		c.set(strconv.Itoa(i), chunk)
	}
	if c.bytes > imageDataCacheBytes {
		t.Fatalf("cache holds %d bytes", c.bytes)
	}
	if _, ok := c.get("0"); ok {
		t.Fatal("oldest entry was not evicted")
	}
	if _, ok := c.get("19"); !ok {
		t.Fatal("newest entry was evicted")
	}
	c.set("too-large", make([]byte, imageDataCacheBytes/16+1))
	if _, ok := c.get("too-large"); ok {
		t.Fatal("too large entry was cached")
	}
}
