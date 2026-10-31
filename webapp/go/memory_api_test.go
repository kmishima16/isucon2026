package isureal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

// ISUREAL_TEST_DSN に空の DB（001-schema.sql と sql/*.sql を流したもの）を渡したときだけ動く
func TestListMemoriesSignsOnlyReturnedPosts(t *testing.T) {
	dsn := os.Getenv("ISUREAL_TEST_DSN")
	if dsn == "" {
		t.Skip("ISUREAL_TEST_DSN is not set")
	}
	db := sqlx.MustOpen("pgx", dsn)
	defer db.Close()
	db.MustExec(`TRUNCATE realmojis, comments, posts, posting_windows, users CASCADE`)
	sum := sha256.Sum256([]byte("memories"))
	keyHash := hex.EncodeToString(sum[:])
	db.MustExec(`INSERT INTO users (id, username, secret_key_hash, display_name, region) VALUES ('u1', 'u1', $1, 'U1', 'asia')`, keyHash)
	db.MustExec(`INSERT INTO posting_windows (id, region, local_date, starts_at, on_time_deadline) VALUES ('w1', 'asia', '2026-10-15', now(), now() + interval '1 day')`)
	base := time.Now().Add(-time.Hour)
	for i := 1; i <= 51; i++ {
		db.MustExec(`INSERT INTO posts (id, author_id, posting_window_id, image_id, visibility, location_kind, posted_at) VALUES ($1, 'u1', 'w1', $2, 'public', 'none', $3)`,
			fmt.Sprintf("p%02d", i), fmt.Sprintf("img-%02d", i), base.Add(time.Duration(i)*time.Second))
	}

	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	response, err := listMemories(context.Background(), db, "u1", start, start.AddDate(0, 1, 0), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 50 || !response.HasMore || response.Items[0].ID != "p51" || response.Items[49].ID != "p02" {
		t.Fatalf("got %d items hasMore=%v", len(response.Items), response.HasMore)
	}
	key, err := imageSigningKey(keyHash)
	if err != nil {
		t.Fatal(err)
	}
	signedURLCache.mu.Lock()
	_, signed := signedURLCache.sigs[string(key)+"img-01"]
	signedURLCache.mu.Unlock()
	if signed {
		t.Fatal("the 51st post is not returned but its image was signed")
	}
}
