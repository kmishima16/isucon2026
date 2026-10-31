package isureal

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v5"
)

// ISUREAL_TEST_DSN に空の DB（001-schema.sql と sql/ を流したもの）を渡したときだけ動く
func TestPostImagesLiveOnTheReceivingServer(t *testing.T) {
	dsn := os.Getenv("ISUREAL_TEST_DSN")
	if dsn == "" {
		t.Skip("ISUREAL_TEST_DSN is not set")
	}
	db := sqlx.MustOpen("pgx", dsn)
	defer db.Close()
	images.dir = t.TempDir()
	cache := newImageDataCache()
	configureImageStore("asia", cache)

	hashOf := func(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) }
	db.MustExec(`TRUNCATE image_object_locations, realmojis, comments, posts, posting_windows, users, image_objects CASCADE`)
	db.MustExec(`INSERT INTO users (id, username, secret_key_hash, display_name, region) VALUES ('u1', 'u1', $1, 'U1', 'asia')`, hashOf("k1"))
	db.MustExec(`INSERT INTO posting_windows (id, region, local_date, starts_at, on_time_deadline) VALUES ('w1', 'asia', '2026-10-31', now() - interval '1 minute', now() + interval '1 day')`)
	key, _ := imageSigningKey(hashOf("k1"))

	// ほかの台の窓口のかわり
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/internal/images/aaaaaaaa-0000-0000-0000-000000000001" {
			w.Write([]byte("REMOTE"))
			return
		}
		http.NotFound(w, r)
	}))
	defer remote.Close()

	h := &HandlerContext{DB: db, Config: &Config{Region: "asia"}, ImageData: cache}
	e := echo.New()
	e.HTTPErrorHandler = APIErrorHandler
	session := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Set("session", &Session{UserID: "u1", Region: "asia"})
			return next(c)
		}
	}
	e.POST("/api/posts", createPostHandler(h), session)
	e.GET("/api/images/:imageId", getImageHandler(h))
	e.GET("/api/me/export", exportPostsHandler(h), session)
	e.POST("/api/initialize", func(c *echo.Context) error { images.purgeAll(c.Request().Context()); return c.NoContent(http.StatusOK) })

	var jpg bytes.Buffer
	jpeg.Encode(&jpg, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil)
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	fw, _ := w.CreateFormFile("image", "a.jpg")
	fw.Write(jpg.Bytes())
	w.WriteField("visibility", "public")
	w.WriteField("location_kind", "none")
	w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/posts", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("post: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &created)
	var imageID string
	db.Get(&imageID, `SELECT image_id FROM posts WHERE id = $1`, created.ID)

	// 中身はこの台のディスクにあり、DB の中身は空で、中身を持つ台が書かれている
	if onDisk, err := os.ReadFile(images.path(imageID)); err != nil || !bytes.Equal(onDisk, jpg.Bytes()) {
		t.Fatalf("image on disk: %v", err)
	}
	var row struct {
		Size int    `db:"size"`
		Host string `db:"host"`
	}
	db.Get(&row, `SELECT octet_length(i.data) AS size, l.host FROM image_objects i JOIN image_object_locations l ON l.id = i.id AND l.created_at = i.created_at WHERE i.id = $1`, imageID)
	if row.Size != 0 || row.Host != images.self {
		t.Fatalf("db row: %+v (self %s)", row, images.self)
	}

	get := func(id string) (int, []byte) {
		exp := time.Now().Add(time.Hour).Unix()
		sig := base64.RawURLEncoding.EncodeToString(imageSignature(key, id, exp))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/images/"+id+"?exp="+strconv.FormatInt(exp, 10)+"&sig="+sig, nil))
		return rec.Code, rec.Body.Bytes()
	}
	cache.reset()
	if code, got := get(imageID); code != http.StatusOK || !bytes.Equal(got, jpg.Bytes()) {
		t.Fatalf("local image: %d", code)
	}

	// ほかの台が持つ画像は、その台の窓口から取り寄せる
	remoteID := "aaaaaaaa-0000-0000-0000-000000000001"
	db.MustExec(`INSERT INTO image_objects (id, owner_user_id, data, media_type, created_at) VALUES ($1, 'u1', '', 'image/jpeg', now() - interval '1 hour')`, remoteID)
	db.MustExec(`INSERT INTO image_object_locations (id, created_at, host) SELECT id, created_at, $2 FROM image_objects WHERE id = $1`, remoteID, remote.URL)
	db.MustExec(`INSERT INTO posts (id, author_id, posting_window_id, image_id, visibility, location_kind, posted_at) SELECT 'p-remote', 'u1', 'w1', id, 'public', 'none', created_at FROM image_objects WHERE id = $1`, remoteID)
	if code, got := get(remoteID); code != http.StatusOK || string(got) != "REMOTE" {
		t.Fatalf("remote image: %d %q", code, got)
	}

	// エクスポートは、この台・ほかの台の両方の画像を集める
	cache.reset()
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/me/export", nil))
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if rec.Code != http.StatusOK || err != nil {
		t.Fatalf("export: %d %v", rec.Code, err)
	}
	var contents []string
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "images/") {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			contents = append(contents, string(b))
		}
	}
	if len(contents) != 2 || contents[0] != "REMOTE" || contents[1] != string(jpg.Bytes()) {
		t.Fatalf("export images: %d files", len(contents))
	}

	// 削除した投稿の画像は、中身がディスクにあっても返さない
	db.MustExec(`UPDATE posts SET deleted_at = now() WHERE id = $1`, created.ID)
	if code, _ := get(imageID); code != http.StatusForbidden {
		t.Fatalf("deleted post image: %d", code)
	}

	// 内部の窓口は、この台の画像を渡し、空にできる
	internal := httptest.NewServer(NewSignerHandler())
	defer internal.Close()
	res, err := http.Get(internal.URL + "/internal/images/" + imageID)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("internal image: %v %v", err, res)
	}
	res.Body.Close()
	if res, _ := http.Get(internal.URL + "/internal/images/..%2Fetc%2Fpasswd"); res.StatusCode != http.StatusNotFound {
		t.Fatalf("path outside the store: %d", res.StatusCode)
	}

	// 初期化で、この台の置き場所とメモリが空になる（ほかの台にも頼む）
	images.peers = []string{internal.URL}
	cache.set(imageID, []byte("x"))
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/initialize", nil))
	if _, err := os.Stat(images.path(imageID)); !os.IsNotExist(err) {
		t.Fatalf("image still on disk after initialize: %v", err)
	}
	if _, ok := cache.get(imageID); ok {
		t.Fatal("cache still holds the image after initialize")
	}
}
