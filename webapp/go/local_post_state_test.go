package isureal

import (
	"testing"
	"time"
)

func TestLocalPostState(t *testing.T) {
	saved := localPostStateEnabled
	localPostStateEnabled = true
	defer func() { localPostStateEnabled = saved }()
	s := &localPostState{known: map[string][]localPost{}}
	day := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	window := &activeWindow{ID: "w1", LocalDate: day}
	// この台で登録していない人は持たない（DB で読む）
	if _, _, ok := s.counts("stranger", "2026-10-31"); ok {
		t.Fatal("knew a user registered elsewhere")
	}
	s.registered("u1")
	if active, deleted, ok := s.counts("u1", "2026-10-31"); !ok || active != 0 || deleted != 0 {
		t.Fatalf("new user: %d %d %v", active, deleted, ok)
	}
	s.posted("u1", "p1", window, "w1")
	s.deleted("u1", "p1")
	s.posted("u1", "p2", window, "w1")
	if active, deleted, _ := s.counts("u1", "2026-10-31"); active != 1 || deleted != 1 {
		t.Fatalf("after post, delete, post: %d %d", active, deleted)
	}
	if active, deleted, _ := s.counts("u1", "2026-11-01"); active != 0 || deleted != 0 {
		t.Fatalf("next day: %d %d", active, deleted)
	}
	// 見張りの投稿期間と投稿の投稿期間が合わないときや、知らない投稿を消したときは、持たない人に戻す
	s.registered("u2")
	s.posted("u2", "p3", window, "other-window")
	if _, _, ok := s.counts("u2", "2026-10-31"); ok {
		t.Fatal("kept a user after a post in an unknown window")
	}
	s.registered("u3")
	s.deleted("u3", "unknown")
	if _, _, ok := s.counts("u3", "2026-10-31"); ok {
		t.Fatal("kept a user after deleting an unknown post")
	}
	s.forget("u1")
	if _, _, ok := s.counts("u1", "2026-10-31"); ok {
		t.Fatal("kept a forgotten user")
	}
	s.registered("u4")
	s.reset()
	if _, _, ok := s.counts("u4", "2026-10-31"); ok {
		t.Fatal("kept a user after reset")
	}
}
