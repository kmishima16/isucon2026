package isureal

import (
	"bytes"
	"encoding/json"
	"image"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v5"
)

// delayProxy は DB への接続を中継し、アプリから DB へ送ったまとまりの数（おおよその往復の数）を数える
type delayProxy struct {
	addr   string
	writes atomic.Int64
}

func startDelayProxy(t *testing.T, target string) *delayProxy {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &delayProxy{addr: ln.Addr().String()}
	go func() {
		for {
			client, err := ln.Accept()
			if err != nil {
				return
			}
			server, err := net.Dial("tcp", target)
			if err != nil {
				client.Close()
				continue
			}
			go func() {
				buf := make([]byte, 64*1024)
				for {
					n, err := client.Read(buf)
					if n > 0 {
						p.writes.Add(1)
						if _, werr := server.Write(buf[:n]); werr != nil {
							break
						}
					}
					if err != nil {
						break
					}
				}
				server.Close()
			}()
			go func() { io.Copy(client, server); client.Close() }()
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return p
}

// ISUREAL_TEST_DSN に空の DB（001-schema.sql と sql/ を流したもの）を渡したときだけ動く
func TestCreatePostInOneRoundTrip(t *testing.T) {
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
	db.MustExec(`TRUNCATE image_object_locations, realmojis, comments, posts, posting_windows, users, image_objects CASCADE`)
	db.MustExec(`INSERT INTO users (id, username, secret_key_hash, display_name, bio, region) VALUES
	  ('u1', 'u1', $1, 'U1', 'hello', 'asia'), ('u2', 'u2', $1, 'U2', '', 'europe'), ('gone', 'gone', $1, 'G', '', 'asia')`, strings.Repeat("0", 64))
	db.MustExec(`UPDATE users SET status = 'deleted' WHERE id = 'gone'`)
	db.MustExec(`INSERT INTO posting_windows (id, region, local_date, starts_at, on_time_deadline) VALUES
	  ('w1', 'asia', '2026-10-31', now() - interval '1 minute', now() + interval '1 day'),
	  ('w2', 'europe', '2026-10-31', now() + interval '1 hour', now() + interval '1 day')`)

	h := &HandlerContext{DB: db, Config: &Config{Region: "asia"}}
	e := echo.New()
	e.HTTPErrorHandler = APIErrorHandler
	e.POST("/api/posts", createPostHandler(h), func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Set("session", &Session{UserID: c.Request().Header.Get("X-Test-User"), Region: "asia"})
			return next(c)
		}
	})
	var jpg bytes.Buffer
	jpeg.Encode(&jpg, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil)
	post := func(user string) (int, map[string]any) {
		var body bytes.Buffer
		w := multipart.NewWriter(&body)
		fw, _ := w.CreateFormFile("image", "a.jpg")
		fw.Write(jpg.Bytes())
		w.WriteField("caption", "hi")
		w.WriteField("visibility", "friends")
		w.WriteField("location_kind", "approximate")
		w.WriteField("locality", "Tokyo")
		w.Close()
		req := httptest.NewRequest(http.MethodPost, "/api/posts", &body)
		req.Header.Set("Content-Type", w.FormDataContentType())
		req.Header.Set("X-Test-User", user)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		var res map[string]any
		json.Unmarshal(rec.Body.Bytes(), &res)
		return rec.Code, res
	}

	db.MustExec(`SELECT 1`)
	before := proxy.writes.Load()
	code, res := post("u1")
	trips := proxy.writes.Load() - before
	if code != http.StatusCreated {
		t.Fatalf("first post: %d %v", code, res)
	}
	t.Logf("round trips for a post: %d", trips)
	if trips > 2 {
		t.Fatalf("post took %d round trips", trips)
	}
	author, _ := res["author"].(map[string]any)
	location, _ := res["location"].(map[string]any)
	if res["caption"] != "hi" || res["visibility"] != "friends" || res["postingWindowId"] != "w1" || res["onTime"] != true ||
		res["lateSeconds"] != float64(0) || author["id"] != "u1" || author["bio"] != "hello" ||
		location["kind"] != "approximate" || location["locality"] != "Tokyo" || !strings.HasPrefix(res["imageUrl"].(string), "/api/images/") {
		t.Fatalf("unexpected response %v", res)
	}
	var stored int
	db.Get(&stored, `SELECT count(*) FROM posts p JOIN image_objects i ON i.id = p.image_id AND i.created_at = p.posted_at JOIN image_object_locations l ON l.id = p.image_id WHERE p.author_id = 'u1'`)
	if stored != 1 {
		t.Fatalf("post, image and location rows: %d", stored)
	}

	if code, res := post("u1"); code != http.StatusConflict || res["error"] != "post_slot_unavailable" {
		t.Fatalf("second post: %d %v", code, res)
	}
	db.MustExec(`UPDATE posts SET deleted_at = now() WHERE author_id = 'u1'`)
	if code, _ := post("u1"); code != http.StatusCreated {
		t.Fatalf("post after one deletion: %d", code)
	}
	db.MustExec(`UPDATE posts SET deleted_at = now() WHERE author_id = 'u1'`)
	if code, res := post("u1"); code != http.StatusConflict || res["error"] != "post_slot_unavailable" {
		t.Fatalf("post after two deletions: %d %v", code, res)
	}
	if code, res := post("u2"); code != http.StatusConflict || res["error"] != "post_slot_unavailable" {
		t.Fatalf("post before the window starts: %d %v", code, res)
	}
	db.MustExec(`UPDATE posting_windows SET ended_at = now() WHERE region = 'asia'`)
	db.MustExec(`UPDATE posts SET deleted_at = NULL`)
	db.MustExec(`DELETE FROM posts`)
	if code, res := post("u1"); code != http.StatusConflict || res["error"] != "post_slot_unavailable" {
		t.Fatalf("post without an open window: %d %v", code, res)
	}
	if code, _ := post("gone"); code != http.StatusInternalServerError {
		t.Fatalf("deleted user: %d", code)
	}
}
