package isureal

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

// ISUREAL_TEST_DSN に空の DB（001-schema.sql と sql/*.sql を流したもの）を渡したときだけ動く
func TestServableImageByReferenceMatchesOriginal(t *testing.T) {
	dsn := os.Getenv("ISUREAL_TEST_DSN")
	if dsn == "" {
		t.Skip("ISUREAL_TEST_DSN is not set")
	}
	db := sqlx.MustOpen("pgx", dsn)
	defer db.Close()
	ctx := context.Background()
	db.MustExec(`TRUNCATE image_object_locations, realmojis, posts, posting_windows, users, image_objects CASCADE`)
	for _, u := range []string{"author", "reactor", "gone", "profiled", "replaced"} {
		db.MustExec(`INSERT INTO users (id, username, secret_key_hash, display_name, region) VALUES ($1, $2, $3, $4, 'asia')`, u, u, strings.Repeat(u[:1], 64), u)
	}
	db.MustExec(`INSERT INTO posting_windows (id, region, local_date, starts_at, on_time_deadline) VALUES ('w1', 'asia', '2026-10-31', now(), now() + interval '1 day')`)
	at := time.Date(2026, 10, 31, 1, 0, 0, 0, time.UTC)
	tx := db.MustBegin()
	for id, owner := range map[string]string{
		"post-live": "author", "post-deleted": "author", "post-gone": "gone",
		"moji-live": "reactor", "moji-replaced": "reactor", "moji-on-deleted": "reactor", "moji-by-gone": "gone",
		"profile-now": "profiled", "profile-old": "replaced", "orphan": "author",
	} {
		if err := insertImageObject(ctx, tx, id, owner, []byte("DATA-"+id), &at); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	db.MustExec(`INSERT INTO posts (id, author_id, posting_window_id, image_id, visibility, location_kind, posted_at, deleted_at) VALUES
	  ('p-live', 'author', 'w1', 'post-live', 'public', 'none', $1, NULL),
	  ('p-deleted', 'author', 'w1', 'post-deleted', 'public', 'none', $1, now()),
	  ('p-gone', 'gone', 'w1', 'post-gone', 'public', 'none', $1, NULL)`, at)
	db.MustExec(`INSERT INTO realmojis (id, post_id, reactor_id, image_id, reaction_type, created_at, replaced_at) VALUES
	  ('r-live', 'p-live', 'reactor', 'moji-live', 'like', now(), NULL),
	  ('r-replaced', 'p-live', 'reactor', 'moji-replaced', 'like', now(), now()),
	  ('r-on-deleted', 'p-deleted', 'reactor', 'moji-on-deleted', 'like', now(), NULL),
	  ('r-by-gone', 'p-live', 'gone', 'moji-by-gone', 'like', now(), NULL)`)
	db.MustExec(`UPDATE users SET profile_image_id = 'profile-now' WHERE id = 'profiled'`)
	// 差し替えたプロフィール画像は、行を消して参照も外す（profile_image_api.go）
	db.MustExec(`UPDATE users SET profile_image_id = NULL WHERE id = 'replaced'`)
	tx = db.MustBegin()
	if err := deleteImageObject(ctx, tx, "profile-old"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	db.MustExec(`UPDATE users SET status = 'deleted' WHERE id = 'gone'`)
	// 置き場所の表に無い画像（古いアプリが書いたもの）
	db.MustExec(`INSERT INTO image_objects (id, owner_user_id, data, media_type) VALUES ('legacy', 'author', 'DATA-legacy', 'image/jpeg')`)
	db.MustExec(`INSERT INTO posts (id, author_id, posting_window_id, image_id, visibility, location_kind, posted_at) VALUES ('p-legacy', 'author', 'w1', 'legacy', 'public', 'none', now())`)

	original := func(id string, skipData bool) (servableImage, error) {
		var row servableImage
		err := sqlx.GetContext(ctx, db, &row, servableImageLocated, id, skipData)
		if errors.Is(err, sql.ErrNoRows) {
			err = sqlx.GetContext(ctx, db, &row, servableImageLegacy, id, skipData)
		}
		return row, err
	}
	for _, id := range []string{"post-live", "post-deleted", "post-gone", "moji-live", "moji-replaced", "moji-on-deleted", "moji-by-gone", "profile-now", "profile-old", "orphan", "legacy", "missing"} {
		for _, skipData := range []bool{false, true} {
			want, wantErr := original(id, skipData)
			got, gotErr := getServableImage(ctx, db, id, skipData)
			if errors.Is(wantErr, sql.ErrNoRows) != errors.Is(gotErr, sql.ErrNoRows) || (wantErr == nil) != (gotErr == nil) {
				t.Fatalf("%s skip=%v: err %v, want %v", id, skipData, gotErr, wantErr)
			}
			if wantErr != nil {
				continue
			}
			if got.SecretKeyHash != want.SecretKeyHash || got.Live != want.Live || got.MediaType != want.MediaType ||
				(got.Host == nil) != (want.Host == nil) || string(got.Data) != string(want.Data) {
				t.Fatalf("%s skip=%v:\n got %+v\nwant %+v", id, skipData, got, want)
			}
		}
	}
}
