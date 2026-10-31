package isureal

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
)

// ISUREAL_TEST_DSN に空の DB（001-schema.sql と sql/*.sql を流したもの）を渡したときだけ動く
func TestSearchSnapshotReflectsChanges(t *testing.T) {
	dsn := os.Getenv("ISUREAL_TEST_DSN")
	if dsn == "" {
		t.Skip("ISUREAL_TEST_DSN is not set")
	}
	db := sqlx.MustOpen("pgx", dsn)
	defer db.Close()
	ctx := context.Background()
	db.MustExec(`TRUNCATE follows, friendships, friend_requests, block_relations, users CASCADE`)
	insert := func(id, username, displayName string) {
		db.MustExec(`INSERT INTO users (id, username, secret_key_hash, display_name, region) VALUES ($1, $2, $3, $4, 'asia')`, id, username, strings.Repeat("0", 64), displayName)
	}
	insert("viewer", "viewer", "Viewer")
	insert("a", "alice", "Alice")
	insert("b", "bob", "Bob Taro")
	s := newSearchSnapshot()
	names := func(q string) string {
		t.Helper()
		users, err := s.search(ctx, db, "viewer", normalizeSearchText(q), "", "", 51)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, u := range users {
			out = append(out, u.Username)
		}
		return strings.Join(out, ",")
	}
	if got := names("taro"); got != "bob" {
		t.Fatalf("before changes: %q", got)
	}
	// 写しを作った後の登録・表示名の変更・退会・ブロックは、次の検索から反映する
	insert("c", "carol", "Carol Taro")
	db.MustExec(`UPDATE users SET display_name = 'Alice Taro', search_display_name = NULL, updated_at = clock_timestamp() WHERE id = 'a'`)
	if got := names("taro"); got != "alice,bob,carol" {
		t.Fatalf("after register and rename: %q", got)
	}
	db.MustExec(`UPDATE users SET status = 'deleted', updated_at = clock_timestamp() WHERE id = 'b'`)
	db.MustExec(`INSERT INTO block_relations (id, blocker_id, blocked_id) VALUES ('blk', 'c', 'viewer')`)
	if got := names("taro"); got != "alice" {
		t.Fatalf("after withdrawal and block: %q", got)
	}
	// 1 秒ごとの取り込みで写しに入れた後も同じ
	if err := s.refresh(ctx, db); err != nil {
		t.Fatal(err)
	}
	if got := names("taro"); got != "alice" {
		t.Fatalf("after refresh: %q", got)
	}
	// 初期化は updated_at を古い値に戻す。世代が変われば読み直す
	db.MustExec(`DELETE FROM block_relations`)
	db.MustExec(`DELETE FROM users WHERE id = 'c'`)
	db.MustExec(`UPDATE users SET display_name = 'Alice', search_display_name = NULL, status = 'active', updated_at = '2020-01-01' WHERE id IN ('a', 'b')`)
	db.MustExec(`UPDATE users SET display_name = 'Bob Taro' WHERE id = 'b'`)
	db.MustExec(`UPDATE app_generation SET n = n + 1 WHERE id = 1`)
	if got := names("taro"); got != "bob" {
		t.Fatalf("after initialize: %q", got)
	}
	// カーソルより後だけを返す
	insert("d", "dave", "Dave Taro")
	users, err := s.search(ctx, db, "viewer", "taro", "bob", "b", 51)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].Username != "dave" {
		t.Fatalf("after cursor: %+v", users)
	}
}
