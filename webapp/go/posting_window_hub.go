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
	// 台のメモリから「今の投稿期間」を返すため（local_post_state.go）
	LocalDate      time.Time `db:"local_date"`
	OnTimeDeadline time.Time `db:"on_time_deadline"`
}

type PostingWindowHub struct {
	mu      sync.Mutex
	current *activeWindow
	// 全員に確かめ直させる世代（初期化）
	userGen uint64
	// その利用者だけに確かめ直させる世代（退会・リージョンの移動）。全員分を上げると、変更のたびにつながっている全員が DB を読みに行く
	userGens map[string]uint64
	changed  chan struct{}
}

func newPostingWindowHub() *PostingWindowHub {
	return &PostingWindowHub{userGens: map[string]uint64{}, changed: make(chan struct{})}
}

// snapshot は今の投稿期間と、次に何か変わったら閉じるチャネルを返す
func (h *PostingWindowHub) snapshot() (*activeWindow, uint64, <-chan struct{}) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.current, h.userGen, h.changed
}

// snapshotFor は、snapshot に加えて、その利用者について確かめ直しが要るかを表す世代を返す
func (h *PostingWindowHub) snapshotFor(userID string) (*activeWindow, uint64, <-chan struct{}) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.current, h.userGen + h.userGens[userID], h.changed
}

func (h *PostingWindowHub) set(window *activeWindow) {
	if window != nil {
		// DB から読んだ時刻とほかの台から渡された時刻を、同じ投稿期間なら同じ値として比べる
		normalized := *window
		normalized.StartsAt, normalized.LocalDate, normalized.OnTimeDeadline = window.StartsAt.UTC(), window.LocalDate.UTC(), window.OnTimeDeadline.UTC()
		window = &normalized
	}
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

// この台で退会やリージョンの変更があったら、その利用者の SSE に自分の状態を確かめ直させる。
// SSE は投稿期間の開始のときに DB を読まず、接続のときのリージョンと登録の時刻で通知を決めるので、この知らせが届かないと古い状態のまま通知してしまう。
// PATCH /api/me は RegionMiddleware を通らず、どの台でも受けうるので、全台に知らせ、届くのを待ってから応答する
func (h *PostingWindowHub) UserChanged(userID string) {
	h.userChangedLocal(userID)
	images.notifyUserChanged(userID)
}

// userChangedLocal は、userID の SSE に確かめ直させる。userID が空なら全員に確かめ直させる
func (h *PostingWindowHub) userChangedLocal(userID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if userID == "" {
		h.userGen++
	} else {
		h.userGens[userID]++
	}
	close(h.changed)
	h.changed = make(chan struct{})
}

func loadActiveWindow(ctx context.Context, db sqlx.QueryerContext, region string) (*activeWindow, error) {
	var window activeWindow
	err := sqlx.GetContext(ctx, db, &window, `SELECT id, region, starts_at, local_date, on_time_deadline FROM posting_windows WHERE region = $1 AND ended_at IS NULL`, region)
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
