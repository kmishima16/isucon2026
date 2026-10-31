package isureal

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v5"
)

// europe1・america1 の読み取りを、同じ台の PostgreSQL（asia2 の hot standby）で済ませる。asia2 までは往復 100ms あり、
// asia に転送すると asia の 2 台の処理能力を 3 リージョンで分け合うことになっていた（europe・america は倍率 1.5 なので、自分の台で読むほうが点になる）。
// 規則は、リージョンをまたいでも書き込みの成功の後の読み取りに結果を反映することを求める。読み取りが届いた後に主の DB で聞いた WAL の位置まで、
// 複製が適用するのを待ってから読む（関所）。間に合わないときや複製が使えないときは、主の DB で読む
type walFence struct {
	primary *sqlx.DB
	replica *sqlx.DB

	mu       sync.Mutex
	probedAt time.Time // 主の DB に WAL の位置を聞いた時刻（この台の時計で、送った時刻）
	probed   uint64
	replayed uint64
	changed  chan struct{}
}

const (
	// 主の DB に WAL の位置を聞く間隔。往復 100ms なので、聞きながら次を聞く
	walProbeInterval = 20 * time.Millisecond
	// 複製の適用の位置を見る間隔（同じ台なので軽い）
	walReplayInterval = 2 * time.Millisecond
	// 関所で待つのはここまで。超えたら主の DB で読む
	walFenceLimit = 250 * time.Millisecond
)

func newWALFence(primary, replica *sqlx.DB) *walFence {
	f := &walFence{primary: primary, replica: replica, changed: make(chan struct{})}
	go f.probeLoop()
	go f.replayLoop()
	return f
}

func (f *walFence) update(apply func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	apply()
	close(f.changed)
	f.changed = make(chan struct{})
}

func (f *walFence) probeLoop() {
	ticker := time.NewTicker(walProbeInterval)
	defer ticker.Stop()
	for range ticker.C {
		go func() {
			sentAt := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			var s string
			if err := f.primary.GetContext(ctx, &s, `SELECT pg_current_wal_insert_lsn()::text`); err != nil {
				return
			}
			lsn, ok := parseLSN(s)
			if !ok {
				return
			}
			f.update(func() {
				if sentAt.After(f.probedAt) {
					f.probedAt, f.probed = sentAt, lsn
				}
			})
		}()
	}
}

func (f *walFence) replayLoop() {
	ticker := time.NewTicker(walReplayInterval)
	defer ticker.Stop()
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		var s sql.NullString
		err := f.replica.GetContext(ctx, &s, `SELECT pg_last_wal_replay_lsn()::text`)
		cancel()
		if err != nil || !s.Valid {
			continue
		}
		lsn, ok := parseLSN(s.String)
		if !ok {
			continue
		}
		f.mu.Lock()
		advanced := lsn > f.replayed
		f.mu.Unlock()
		if advanced {
			f.update(func() { f.replayed = lsn })
		}
	}
}

// wait は、arrivedAt より後に主の DB で聞いた WAL の位置まで、複製が適用するのを待つ。walFenceLimit までに届けば true
func (f *walFence) wait(ctx context.Context, arrivedAt time.Time) bool {
	deadline := time.NewTimer(walFenceLimit)
	defer deadline.Stop()
	var target uint64
	haveTarget := false
	for {
		f.mu.Lock()
		if !haveTarget && f.probedAt.After(arrivedAt) {
			target, haveTarget = f.probed, true
		}
		ok := haveTarget && f.replayed >= target
		changed := f.changed
		f.mu.Unlock()
		if ok {
			return true
		}
		select {
		case <-changed:
		case <-deadline.C:
			return false
		case <-ctx.Done():
			return false
		}
	}
}

// parseLSN は、PostgreSQL の WAL の位置（例：16/B374D848）を数に直す
func parseLSN(s string) (uint64, bool) {
	hi, lo, found := strings.Cut(s, "/")
	if !found {
		return 0, false
	}
	h, err := strconv.ParseUint(hi, 16, 32)
	if err != nil {
		return 0, false
	}
	l, err := strconv.ParseUint(lo, 16, 32)
	if err != nil {
		return 0, false
	}
	return h<<32 | l, true
}

// replicaReadRoutes は、読み取りの API を、関所を通れば複製で、通れなければ主の DB で処理する。
// 複製を使わない台（asia1・asia2）では、今までどおり主の DB で処理する
type replicaReadRoutes struct {
	e       *echo.Echo
	primary *HandlerContext
	replica *HandlerContext
	fence   *walFence
}

func (r *replicaReadRoutes) GET(path string, handler func(*HandlerContext) echo.HandlerFunc) {
	onPrimary := SessionMiddleware(r.primary, true)(RegionMiddleware(r.primary)(handler(r.primary)))
	if r.fence == nil {
		r.e.GET(path, onPrimary)
		return
	}
	onReplica := SessionMiddleware(r.replica, true)(RegionMiddleware(r.replica)(handler(r.replica)))
	r.e.GET(path, func(c *echo.Context) error {
		if r.fence.wait(c.Request().Context(), time.Now()) {
			return onReplica(c)
		}
		return onPrimary(c)
	})
}

// openReplicaReads は、ISUREAL_REPLICA_HOST があれば、その台の複製と関所を用意する
func openReplicaReads(e *echo.Echo, config *Config, primary *HandlerContext) *replicaReadRoutes {
	routes := &replicaReadRoutes{e: e, primary: primary, replica: primary}
	host := envOrDefault("ISUREAL_REPLICA_HOST", "")
	if host == "" {
		return routes
	}
	replicaConfig := *config
	replicaConfig.DBHost = host
	replicaConfig.DBSSLMode = "disable"
	replicaDB, err := sqlx.Open("pgx", replicaConfig.DatabaseDSN())
	if err != nil {
		panic("failed to open replica: " + err.Error())
	}
	replicaDB.SetMaxOpenConns(60)
	replicaDB.SetMaxIdleConns(60)
	// WAL の位置を聞く接続は、アプリの接続とは分ける（往復 100ms の間ふさがるので、アプリの接続を取り合わない）
	probeDB, err := sqlx.Open("pgx", config.DatabaseDSN())
	if err != nil {
		panic("failed to open probe connection: " + err.Error())
	}
	probeDB.SetMaxOpenConns(8)
	probeDB.SetMaxIdleConns(8)
	probeDB.SetConnMaxIdleTime(0)
	replica := *primary
	replica.DB = replicaDB
	// 読みだけのハンドラーは ReadDB で読む（asia1 の複製、1a24c58）。差し替えないと、関所で待った後も往復 100ms 先の主の DB を読む
	replica.ReadDB = replicaDB
	routes.replica = &replica
	routes.fence = newWALFence(probeDB, replicaDB)
	return routes
}
