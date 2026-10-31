package isureal

import (
	"testing"
	"time"
)

func TestSessionCache(t *testing.T) {
	c := &sessionCache{entries: make(map[string]sessionCacheEntry)}
	c.set(&Session{UserID: "u1", TokenHash: "t1", Region: "europe"})
	c.set(&Session{UserID: "u2", TokenHash: "t2", Region: "europe"})
	if s, ok := c.get("t1"); !ok || s.UserID != "u1" || s.Region != "europe" {
		t.Fatalf("get t1 = %+v %v", s, ok)
	}
	// ログアウト・退会・リージョンの変更・ログインし直しでは、その人のセッションをすぐ忘れる
	c.forgetUser("u1")
	if _, ok := c.get("t1"); ok {
		t.Fatal("kept a forgotten user's session")
	}
	if _, ok := c.get("t2"); !ok {
		t.Fatal("forgot another user's session")
	}
	// ほかの台で無効になったセッションは、期限が過ぎたら DB で読み直す
	c.entries["t2"] = sessionCacheEntry{session: c.entries["t2"].session, cachedAt: time.Now().Add(-sessionCacheTTL)}
	if _, ok := c.get("t2"); ok {
		t.Fatal("returned an expired session")
	}
	c.set(&Session{UserID: "u3", TokenHash: "t3"})
	c.reset()
	if _, ok := c.get("t3"); ok {
		t.Fatal("kept a session after reset")
	}
	// 無効の台（nil）では何も覚えない
	var off *sessionCache
	off.set(&Session{UserID: "u4", TokenHash: "t4"})
	if _, ok := off.get("t4"); ok {
		t.Fatal("nil cache returned a session")
	}
}
