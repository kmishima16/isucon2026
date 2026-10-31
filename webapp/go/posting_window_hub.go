package isureal

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// 通知は投稿期間の開始で全員分を INSERT せず、台ごとに今の投稿期間を見張って、つながっている利用者に配る。
// 全員分の INSERT は約 4900 行で 0.6 秒かかり、SSE は接続ごとに 500ms おきに DB を見ていたので、通知が 1 秒ほど遅れて投稿期間を削っていた
const postingWindowPollInterval = 50 * time.Millisecond

var notificationNamespace = uuid.MustParse("6f1d3c52-4f0e-4b8a-9a43-0f7f3b1e2d5a")

// 通知 ID は利用者と投稿期間から決める。再接続しても同じ通知には同じ ID を返すため
func notificationID(userID, postingWindowID string) string {
	return uuid.NewSHA1(notificationNamespace, []byte(postingWindowID+":"+userID)).String()
}

type activeWindow struct {
	ID       string    `db:"id"`
	Region   string    `db:"region"`
	StartsAt time.Time `db:"starts_at"`
}

type PostingWindowHub struct {
	mu      sync.Mutex
	current *activeWindow
	userGen uint64
	changed chan struct{}
}

func newPostingWindowHub() *PostingWindowHub {
	return &PostingWindowHub{changed: make(chan struct{})}
}

// snapshot は今の投稿期間と、次に何か変わったら閉じるチャネルを返す
func (h *PostingWindowHub) snapshot() (*activeWindow, uint64, <-chan struct{}) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.current, h.userGen, h.changed
}

func (h *PostingWindowHub) set(window *activeWindow) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if window == nil && h.current == nil {
		return
	}
	if window != nil && h.current != nil && *window == *h.current {
		return
	}
	h.current = window
	close(h.changed)
	h.changed = make(chan struct{})
}

// 内部の窓口（/internal/user-changed）が、ほかの台での変更をこの台の利用者に伝えるために使う
var localWindowHub atomic.Pointer[PostingWindowHub]

// この台で退会やリージョンの変更があったら、つながっている利用者に自分の状態を確かめ直させる。
// asia は asia1 と asia2 の 2 台で受けるので、SSE がもう 1 台につながっている利用者にも伝える
func (h *PostingWindowHub) UserChanged() {
	h.userChangedLocal()
	images.notifyUserChanged()
}

func (h *PostingWindowHub) userChangedLocal() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.userGen++
	close(h.changed)
	h.changed = make(chan struct{})
}

func loadActiveWindow(ctx context.Context, db sqlx.QueryerContext, region string) (*activeWindow, error) {
	var window activeWindow
	err := sqlx.GetContext(ctx, db, &window, `SELECT id, region, starts_at FROM posting_windows WHERE region = $1 AND ended_at IS NULL`, region)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &window, nil
}

// poll はほかの台が開いた投稿期間と、初期化で戻った投稿期間を拾う
func (h *PostingWindowHub) poll(db *sqlx.DB, region string) {
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		window, err := loadActiveWindow(ctx, db, region)
		cancel()
		if err != nil {
			log.Printf("failed to poll posting window: %v", err)
		} else {
			h.set(window)
		}
		time.Sleep(postingWindowPollInterval)
	}
}
