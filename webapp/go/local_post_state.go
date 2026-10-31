package isureal

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// europe1・america1 では、投稿の前に呼ぶ「今の投稿期間」（平均 0.57 秒）と「自分の今日の投稿」（平均 1.46 秒）が、新しい人の経路でいちばん遅かった。
// 新しい人は、asia2 の DB が混む日に投稿が期間に間に合わず（33〜40%）、翌日の新規が 0 人になっていた。
// この台で登録した人の投稿は、リージョンの変更か退会までは、すべてこの台で書かれる（RegionMiddleware がほかの台での書き込みを断る）。
// なので、その人たちの今日の投稿をこの台のメモリに持ち、投稿期間は台の見張り（PostingWindowHub）から返して、DB に聞かない。
// 初期データの利用者や、リージョンを移ってきた人など、この台で登録していない人は、今までどおり DB で読む
var localPostStateEnabled = os.Getenv("ISUREAL_LOCAL_POST_STATE") == "1"

type localPost struct {
	id        string
	localDate string
	deleted   bool
}

type localPostState struct {
	mu    sync.Mutex
	known map[string][]localPost
}

var localPosts = &localPostState{known: map[string][]localPost{}}

// localWindow は、台のメモリで返す台でだけ、見張りの今の投稿期間を返す
func (h *HandlerContext) localWindow() *activeWindow {
	if !localPostStateEnabled || h.Windows == nil {
		return nil
	}
	window, _, _ := h.Windows.snapshot()
	return window
}

func (s *localPostState) registered(userID string) {
	if !localPostStateEnabled {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.known[userID] = nil
}

// forget は、その人をこの台で持たない人に戻す（リージョンの変更・退会・持っている状態と合わない書き込み）
func (s *localPostState) forget(userID string) {
	if !localPostStateEnabled {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.known, userID)
}

func (s *localPostState) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.known = map[string][]localPost{}
}

// posted は、この台で受けた投稿を足す。投稿期間の日付が分からなければ、その人を持たない人に戻す
func (s *localPostState) posted(userID, postID string, window *activeWindow, postingWindowID string) {
	if !localPostStateEnabled {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	posts, ok := s.known[userID]
	if !ok {
		return
	}
	if window == nil || window.ID != postingWindowID {
		delete(s.known, userID)
		return
	}
	s.known[userID] = append(posts, localPost{id: postID, localDate: window.LocalDate.Format(time.DateOnly)})
}

func (s *localPostState) deleted(userID, postID string) {
	if !localPostStateEnabled {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	posts, ok := s.known[userID]
	if !ok {
		return
	}
	for i := range posts {
		if posts[i].id == postID {
			posts[i].deleted = true
			return
		}
	}
	delete(s.known, userID)
}

// counts は、その人の localDate の日の投稿のうち、生きている数と消した数を返す。この台で持っていない人なら ok が false
func (s *localPostState) counts(userID, localDate string) (active, deleted int, ok bool) {
	if !localPostStateEnabled {
		return 0, 0, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	posts, ok := s.known[userID]
	if !ok {
		return 0, 0, false
	}
	for _, post := range posts {
		if post.localDate != localDate {
			continue
		}
		if post.deleted {
			deleted++
		} else {
			active++
		}
	}
	return active, deleted, true
}

// 投稿期間の開始は asia1 が受ける。ほかの台の見張りは 50ms ごとの読み直しと DB までの往復の分だけ遅れるので、
// 開いた投稿期間を、応答を返す前にそのリージョンの台へ直接渡す（台の見張りから投稿期間を返すため）
type windowPush struct {
	ID             string    `json:"id"`
	Region         string    `json:"region"`
	LocalDate      time.Time `json:"localDate"`
	StartsAt       time.Time `json:"startsAt"`
	OnTimeDeadline time.Time `json:"onTimeDeadline"`
}

func pushWindowToRegion(ctx context.Context, window *activeWindow) {
	body, err := json.Marshal(windowPush{ID: window.ID, Region: window.Region, LocalDate: window.LocalDate, StartsAt: window.StartsAt, OnTimeDeadline: window.OnTimeDeadline})
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for host, region := range appImageHosts {
		if host == images.self || region != window.Region {
			continue
		}
		wg.Go(func() {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, host+"/internal/window", bytes.NewReader(body))
			if err != nil {
				return
			}
			res, err := imageHTTPClient.Do(req)
			if err != nil {
				log.Printf("failed to push posting window to %s: %v", host, err)
				return
			}
			_, _ = io.Copy(io.Discard, res.Body)
			res.Body.Close()
		})
	}
	wg.Wait()
}

func registerWindowPushHandler(mux *http.ServeMux) {
	mux.HandleFunc("POST /internal/window", func(w http.ResponseWriter, r *http.Request) {
		if !fromPrivateNetwork(r.RemoteAddr) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var push windowPush
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&push); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if hub := localWindowHub.Load(); hub != nil {
			hub.set(&activeWindow{ID: push.ID, Region: push.Region, LocalDate: push.LocalDate, StartsAt: push.StartsAt, OnTimeDeadline: push.OnTimeDeadline})
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
