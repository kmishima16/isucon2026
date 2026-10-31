package isureal

import (
	"sync"
	"time"
)

// europe1・america1 から DB（asia2）までは 1 往復 100ms あり、ログインが要るリクエストはどれも最初のセッションの確認で 100ms を使っていた。
// 新しい人の経路（検索・友達申請・投稿期間・投稿・友達フィード）が期間の終わりに間に合わず、europe・america の人数が翌日に増えなかった。
// 確かめたセッションを台のメモリに短い間だけ覚える。この台でのログアウト・退会・リージョンの変更・ログインの置き換えは、その場で忘れる。
// ほかの台で無効になったセッション（同じ人がほかの台でログインし直したときなど）は、覚えてから sessionCacheTTL のうちに読み直す
const sessionCacheTTL = 3 * time.Second

type sessionCacheEntry struct {
	session  Session
	cachedAt time.Time
}

type sessionCache struct {
	mu      sync.Mutex
	entries map[string]sessionCacheEntry
}

// sessions は ISUREAL_SESSION_CACHE=1 の台（europe1・america1）でだけ使う。asia1 と asia2 の間は往復 0.4ms で、覚える得が無い
var sessions *sessionCache

func configureSessionCache() {
	if envOrDefault("ISUREAL_SESSION_CACHE", "") == "1" {
		sessions = &sessionCache{entries: make(map[string]sessionCacheEntry)}
	} else {
		sessions = nil
	}
}

func (c *sessionCache) get(tokenHash string) (*Session, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[tokenHash]
	if !ok {
		return nil, false
	}
	if time.Since(entry.cachedAt) >= sessionCacheTTL {
		delete(c.entries, tokenHash)
		return nil, false
	}
	session := entry.session
	return &session, true
}

func (c *sessionCache) set(session *Session) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	// 期限の切れたものを掃除する。この台の利用者は数十人なので、全件を見ても軽い
	for key, entry := range c.entries {
		if now.Sub(entry.cachedAt) >= sessionCacheTTL {
			delete(c.entries, key)
		}
	}
	c.entries[session.TokenHash] = sessionCacheEntry{session: *session, cachedAt: now}
}

// forgetUser は、その人のセッションをすべて忘れる（ログアウト・退会・リージョンの変更・ログインし直し）
func (c *sessionCache) forgetUser(userID string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, entry := range c.entries {
		if entry.session.UserID == userID {
			delete(c.entries, key)
		}
	}
}

// reset は、すべて忘れる（初期化）
func (c *sessionCache) reset() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]sessionCacheEntry)
}
