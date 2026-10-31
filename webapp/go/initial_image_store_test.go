package isureal

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
)

// ISUREAL_TEST_DSN に空の DB（001-schema.sql と sql/*.sql を流したもの）を渡したときだけ動く
func TestExtractInitialImages(t *testing.T) {
	dsn := os.Getenv("ISUREAL_TEST_DSN")
	if dsn == "" {
		t.Skip("ISUREAL_TEST_DSN is not set")
	}
	db := sqlx.MustOpen("pgx", dsn)
	defer db.Close()
	db.MustExec(`TRUNCATE realmojis, comments, posts, posting_windows, users, image_objects CASCADE`)
	db.MustExec(`CREATE TABLE IF NOT EXISTS initial_data_boundary (created_at timestamptz)`)
	db.MustExec(`TRUNCATE initial_data_boundary`)
	db.MustExec(`INSERT INTO initial_data_boundary VALUES ('2025-03-01 12:00:00+00')`)
	db.MustExec(`INSERT INTO users (id, username, secret_key_hash, display_name, region) VALUES ('u1', 'u1', $1, 'U1', 'asia')`, strings.Repeat("0", 64))
	db.MustExec(`INSERT INTO posting_windows (id, region, local_date, starts_at, on_time_deadline, ended_at) VALUES
	  ('w1', 'asia', '2025-03-01', '2025-03-01 00:00:00+00', '2025-03-01 00:02:00+00', '2025-03-02 00:00:00+00')`)
	const old, fresh = "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002"
	db.MustExec(`INSERT INTO image_objects (id, owner_user_id, data, media_type, created_at) VALUES
	  ($1, 'u1', 'INITIAL', 'image/jpeg', '2025-03-01 00:01:00+00'),
	  ($2, 'u1', 'DURING_RUN', 'image/jpeg', '2025-03-01 13:00:00+00')`, old, fresh)
	db.MustExec(`INSERT INTO posts (id, author_id, posting_window_id, image_id, visibility, location_kind, posted_at) VALUES
	  ('p1', 'u1', 'w1', $1, 'public', 'none', '2025-03-01 00:01:00+00'),
	  ('p2', 'u1', 'w1', $2, 'public', 'none', '2025-03-01 13:00:00+00')`, old, fresh)

	s := &initialImageStore{dir: t.TempDir()}
	if _, ok := s.read(old); ok {
		t.Fatal("read before extraction")
	}
	s.checkedAt.Store(0)
	if err := s.extract(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if data, ok := s.read(old); !ok || string(data) != "INITIAL" {
		t.Fatalf("initial image = %q %v", data, ok)
	}
	// 走行中の投稿の画像は書き出さない（初期化で消えるため）
	if _, err := os.Stat(s.path(fresh)); err == nil {
		t.Fatal("wrote an image posted after the boundary")
	}
	if got := s.readMany([]string{old, fresh}); len(got) != 1 || string(got[old]) != "INITIAL" {
		t.Fatalf("readMany = %v", got)
	}
	// 2 回目は書き出し済みのファイルを読み直さない
	if err := os.WriteFile(s.path(old), []byte("KEPT"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.extract(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(s.path(old)); string(data) != "KEPT" {
		t.Fatalf("rewrote an extracted image: %q", data)
	}
	if _, err := os.Stat(filepath.Join(s.dir, initialImageDoneMarker)); err != nil {
		t.Fatal("no done marker")
	}
}

func TestInitialImageStoreNeedsMarker(t *testing.T) {
	s := &initialImageStore{dir: t.TempDir()}
	const id = "00000000-0000-0000-0000-000000000003"
	if err := s.writeFile(id, []byte("X")); err != nil {
		t.Fatal(err)
	}
	// 書き出しを終えた印が無い間は、書きかけかもしれないファイルを読まない
	if _, ok := s.read(id); ok {
		t.Fatal("read without the done marker")
	}
	if err := os.WriteFile(filepath.Join(s.dir, initialImageDoneMarker), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s.checkedAt.Store(0)
	if data, ok := s.read(id); !ok || string(data) != "X" {
		t.Fatalf("read = %q %v", data, ok)
	}
}
