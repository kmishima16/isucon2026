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

// ISUREAL_TEST_DSN に空の DB（001-schema.sql と sql/005_image_object_locations.sql を流したもの）を渡したときだけ動く
func TestImageObjectLocations(t *testing.T) {
	dsn := os.Getenv("ISUREAL_TEST_DSN")
	if dsn == "" {
		t.Skip("ISUREAL_TEST_DSN is not set")
	}
	db := sqlx.MustOpen("pgx", dsn)
	defer db.Close()
	ctx := context.Background()
	db.MustExec(`TRUNCATE image_object_locations, realmojis, posts, posting_windows, users, image_objects CASCADE`)
	db.MustExec(`INSERT INTO users (id, username, secret_key_hash, display_name, region) VALUES ('u1', 'u1', $1, 'U1', 'asia'), ('gone', 'gone', $1, 'G', 'asia')`, strings.Repeat("0", 64))
	db.MustExec(`UPDATE users SET status = 'deleted' WHERE id = 'gone'`)

	postedAt := time.Date(2025, 3, 15, 12, 0, 0, 0, time.UTC)
	tx := db.MustBegin()
	if err := insertImageObject(ctx, tx, "with-time", "u1", []byte("A"), &postedAt); err != nil {
		t.Fatal(err)
	}
	if err := insertImageObject(ctx, tx, "now", "u1", []byte("B"), nil); err != nil {
		t.Fatal(err)
	}
	if err := insertImageObject(ctx, tx, "gone-image", "gone", []byte("C"), nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	// 表に無い画像（古いアプリが書いた画像）
	db.MustExec(`INSERT INTO image_objects (id, owner_user_id, data, media_type) VALUES ('legacy', 'u1', 'D', 'image/jpeg')`)

	var located int
	db.Get(&located, `SELECT count(*) FROM image_object_locations l JOIN image_objects i ON i.id = l.id AND i.created_at = l.created_at`)
	if located != 3 {
		t.Fatalf("locations written with images: %d", located)
	}
	key, err := getImageOwnerKey(ctx, db, "with-time")
	if err != nil || !key.CreatedAt.Equal(postedAt) {
		t.Fatalf("located image: %+v %v", key, err)
	}
	if key, err := getImageOwnerKey(ctx, db, "legacy"); err != nil || key.SecretKeyHash == "" {
		t.Fatalf("image without location: %+v %v", key, err)
	}
	for _, id := range []string{"gone-image", "missing"} {
		if _, err := getImageOwnerKey(ctx, db, id); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("%s: %v", id, err)
		}
	}

	tx = db.MustBegin()
	for _, id := range []string{"now", "legacy"} {
		if err := deleteImageObject(ctx, tx, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var left int
	db.Get(&left, `SELECT count(*) FROM image_objects WHERE id IN ('now', 'legacy')`)
	if left != 0 {
		t.Fatalf("%d images left after delete", left)
	}
}
