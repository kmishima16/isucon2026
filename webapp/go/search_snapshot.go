package isureal

import (
	"cmp"
	"context"
	"database/sql"
	"log"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
)

// ユーザー検索は、候補を絞る SQL が 1 回 2236 ブロックを読み、asia2 の DB の時間の 2 位だった（2540 回・86 秒）。
// 利用者の写し（約 6000 行）を台のメモリに持って照合し、DB には検索のたびに「写しより後に変わった行」とブロックと世代を 1 回で聞く。
// 変わった行を毎回 DB から読むので、ほかの台での登録・表示名の変更・退会も、その検索から反映される（書き込みの後の読み取り）。
// 初期化は updated_at を古い値に戻すので、差分では気付けない。初期化のたびに増やす世代（sql/013_search_memory.sql）が変わったら、写しを読み直す。
// 並びは DB と同じ (username, id) の順。DB の照合順序は C.UTF-8 で、Go の文字列の比べ方と同じ
var searchUsersInMemory = os.Getenv("ISUREAL_SEARCH_MEMORY") == "1"

// 書き込みの updated_at は、コミットより少し前の時刻になる。負荷の山でコミットが遅れた行を取りこぼさないよう、差分はこの分だけ重ねて読む
const searchSnapshotOverlap = 5 * time.Second

type searchEntry struct {
	user        User
	nameNorm    string
	displayNorm string
}

type searchSnapshot struct {
	mu     sync.RWMutex
	ready  bool
	gen    int64
	at     time.Time // DB の時計で、この時刻より前に書かれた行は写しに入っている
	sorted []*searchEntry
	byID   map[string]*searchEntry
}

func newSearchSnapshot() *searchSnapshot {
	return &searchSnapshot{byID: map[string]*searchEntry{}}
}

type searchUserRow struct {
	ID                sql.NullString `db:"id"`
	Username          sql.NullString `db:"username"`
	SecretKeyHash     sql.NullString `db:"secret_key_hash"`
	DisplayName       sql.NullString `db:"display_name"`
	Bio               sql.NullString `db:"bio"`
	ProfileImageID    sql.NullString `db:"profile_image_id"`
	Region            sql.NullString `db:"region"`
	AccountType       sql.NullString `db:"account_type"`
	Status            sql.NullString `db:"status"`
	SearchUsername    sql.NullString `db:"search_username"`
	SearchDisplayName sql.NullString `db:"search_display_name"`
}

func (r searchUserRow) entry() *searchEntry {
	user := User{
		ID: r.ID.String, Username: r.Username.String, SecretKeyHash: r.SecretKeyHash.String, DisplayName: r.DisplayName.String,
		Bio: r.Bio.String, Region: r.Region.String, AccountType: r.AccountType.String, Status: r.Status.String,
	}
	if r.ProfileImageID.Valid {
		id := r.ProfileImageID.String
		user.ProfileImageID = &id
	}
	entry := &searchEntry{user: user, nameNorm: r.SearchUsername.String, displayNorm: r.SearchDisplayName.String}
	if !r.SearchUsername.Valid {
		entry.nameNorm = normalizeSearchText(user.Username)
	}
	if !r.SearchDisplayName.Valid {
		entry.displayNorm = normalizeSearchText(user.DisplayName)
	}
	return entry
}

const searchUserColumns = `u.id, u.username, u.secret_key_hash, u.display_name, u.bio, u.profile_image_id, u.region, u.account_type, u.status, u.search_username, u.search_display_name`

func searchEntryLess(a, b *searchEntry) int {
	if c := cmp.Compare(a.user.Username, b.user.Username); c != 0 {
		return c
	}
	return cmp.Compare(a.user.ID, b.user.ID)
}

// rebuild は、写しを全部読み直す
func (s *searchSnapshot) rebuild(ctx context.Context, db sqlx.QueryerContext) error {
	var head struct {
		Gen int64     `db:"gen"`
		At  time.Time `db:"at"`
	}
	// 世代と時刻を、行より先に読む。行を読む間に初期化が挟まっても、写しの世代は古いままなので、次の検索で読み直す。
	// 行を読む間に書かれた行は、時刻より後なので次の差分で読む
	if err := sqlx.GetContext(ctx, db, &head, `SELECT (SELECT n FROM app_generation WHERE id = 1) AS gen, now() AS at`); err != nil {
		return err
	}
	var rows []searchUserRow
	if err := sqlx.SelectContext(ctx, db, &rows, `SELECT `+searchUserColumns+` FROM users u ORDER BY u.username, u.id`); err != nil {
		return err
	}
	sorted := make([]*searchEntry, 0, len(rows))
	byID := make(map[string]*searchEntry, len(rows))
	for _, row := range rows {
		entry := row.entry()
		sorted = append(sorted, entry)
		byID[entry.user.ID] = entry
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sorted, s.byID, s.gen, s.ready = sorted, byID, head.Gen, true
	s.at = head.At.Add(-searchSnapshotOverlap)
	return nil
}

// merge は、差分の行を写しに入れる。username は変わらないので、ある行は置き換え、無い行は並びの位置に差し込む
func (s *searchSnapshot) merge(entries []*searchEntry, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, entry := range entries {
		if old, ok := s.byID[entry.user.ID]; ok {
			*old = *entry
			continue
		}
		i, _ := slices.BinarySearchFunc(s.sorted, entry, searchEntryLess)
		s.sorted = slices.Insert(s.sorted, i, entry)
		s.byID[entry.user.ID] = entry
	}
	if at.After(s.at) {
		s.at = at
	}
}

type searchDelta struct {
	gen     int64
	at      time.Time
	blocked map[string]bool
	changed []*searchEntry
}

// loadDelta は、世代・DB の時刻・閲覧者のブロック・since より後に変わった行を 1 回で読む
func loadSearchDelta(ctx context.Context, db sqlx.QueryerContext, viewer string, since time.Time) (searchDelta, error) {
	var rows []struct {
		Gen     int64     `db:"gen"`
		At      time.Time `db:"at"`
		Blocked string    `db:"blocked"`
		searchUserRow
	}
	err := sqlx.SelectContext(ctx, db, &rows, `
  SELECT g.gen, g.at, g.blocked, `+searchUserColumns+`
  FROM (
    SELECT (SELECT n FROM app_generation WHERE id = 1) AS gen, now() AS at,
           (SELECT coalesce(string_agg(CASE WHEN blocker_id = $1 THEN blocked_id ELSE blocker_id END, ','), '')
            FROM block_relations WHERE blocker_id = $1 OR blocked_id = $1) AS blocked
  ) g
  LEFT JOIN users u ON u.updated_at > $2
 `, viewer, since)
	if err != nil {
		return searchDelta{}, err
	}
	delta := searchDelta{blocked: map[string]bool{}}
	for i, row := range rows {
		if i == 0 {
			delta.gen, delta.at = row.Gen, row.At
			if row.Blocked != "" {
				for _, id := range strings.Split(row.Blocked, ",") {
					delta.blocked[id] = true
				}
			}
		}
		if row.ID.Valid {
			delta.changed = append(delta.changed, row.entry())
		}
	}
	return delta, nil
}

// search は、閲覧者から見える当たりを (username, id) の順に最大 limit 人返す
func (s *searchSnapshot) search(ctx context.Context, db sqlx.QueryerContext, viewer, query, beforeUsername, beforeID string, limit int) ([]User, error) {
	s.mu.RLock()
	ready, since := s.ready, s.at
	s.mu.RUnlock()
	if !ready {
		since = time.Time{}
	}
	delta, err := loadSearchDelta(ctx, db, viewer, since)
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	stale := !s.ready || s.gen != delta.gen
	s.mu.RUnlock()
	if stale {
		if err := s.rebuild(ctx, db); err != nil {
			return nil, err
		}
		if delta, err = loadSearchDelta(ctx, db, viewer, s.snapshotAt()); err != nil {
			return nil, err
		}
	}
	changed := make(map[string]*searchEntry, len(delta.changed))
	for _, entry := range delta.changed {
		changed[entry.user.ID] = entry
	}
	visible := func(entry *searchEntry) bool {
		if entry.user.Status != "active" || entry.user.ID == viewer || delta.blocked[entry.user.ID] {
			return false
		}
		if beforeUsername != "" && (entry.user.Username < beforeUsername || (entry.user.Username == beforeUsername && entry.user.ID <= beforeID)) {
			return false
		}
		return strings.Contains(entry.nameNorm, query) || strings.Contains(entry.displayNorm, query)
	}
	matched := make([]*searchEntry, 0, limit+len(changed))
	s.mu.RLock()
	// 写しは並んでいるので、写しの当たりは先頭の limit 人だけ見ればよい（差分の当たりと合わせて並べ直す）
	for _, entry := range s.sorted {
		if _, ok := changed[entry.user.ID]; ok {
			continue
		}
		if visible(entry) {
			copied := *entry
			matched = append(matched, &copied)
			if len(matched) == limit {
				break
			}
		}
	}
	s.mu.RUnlock()
	for _, entry := range delta.changed {
		if visible(entry) {
			matched = append(matched, entry)
		}
	}
	slices.SortFunc(matched, searchEntryLess)
	if len(matched) > limit {
		matched = matched[:limit]
	}
	users := make([]User, len(matched))
	for i, entry := range matched {
		users[i] = entry.user
	}
	// 次の検索の差分を小さくするため、読んだ差分を写しに入れる
	if len(delta.changed) > 0 {
		s.merge(delta.changed, delta.at.Add(-searchSnapshotOverlap))
	}
	return users, nil
}

func (s *searchSnapshot) snapshotAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.at
}

// refreshLoop は、写しを 1 秒ごとに差分で進め、検索のたびに読む差分を小さく保つ
func (s *searchSnapshot) refreshLoop(db *sqlx.DB) {
	for {
		time.Sleep(time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := s.refresh(ctx, db); err != nil {
			log.Printf("failed to refresh search snapshot: %v", err)
		}
		cancel()
	}
}

func (s *searchSnapshot) refresh(ctx context.Context, db sqlx.QueryerContext) error {
	s.mu.RLock()
	ready, since := s.ready, s.at
	s.mu.RUnlock()
	if !ready {
		return s.rebuild(ctx, db)
	}
	delta, err := loadSearchDelta(ctx, db, "", since)
	if err != nil {
		return err
	}
	s.mu.RLock()
	stale := s.gen != delta.gen
	s.mu.RUnlock()
	if stale {
		return s.rebuild(ctx, db)
	}
	s.merge(delta.changed, delta.at.Add(-searchSnapshotOverlap))
	return nil
}
