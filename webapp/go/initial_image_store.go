package isureal

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jmoiron/sqlx"
)

// 初期データの投稿の画像（約 62 万枚・約 9.5GB）は、DB の台のメモリ（4GB）に載らず、エクスポートのたびに asia2 のディスクから TOAST を読んでいた（1 回平均 582ms、DB の時間の 1 位）。
// 初期データの画像は id ごとに中身が変わらず、初期化（reset.sql）でも消えないので、ベンチの外で一度だけアプリの台のディスクに書き出し（extract-initial-images）、そこから読む。
// 見せてよいかの判断（署名・削除・退会）は今までどおり毎回 DB で行う。ファイルが無い画像や、書き出しを終えていない台は、今までどおり DB から読む。
// 手順は logs/notes/initial-images-runbook.md

var initialImageDir = envOrDefault("ISUREAL_INITIAL_IMAGE_DIR", "/home/isucon/initial-images")

// 書き出しを最後まで終え、ディスクに確定させた印。これが無い間は、書きかけのファイルを読まない
const initialImageDoneMarker = ".done"

type initialImageStore struct {
	dir   string
	ready atomic.Bool
	// 印を探すのは、見つかるまで 1 秒に 1 回まで
	checkedAt atomic.Int64
}

var initialImages = &initialImageStore{dir: initialImageDir}

func (s *initialImageStore) path(id string) string {
	return filepath.Join(s.dir, id[:2], id)
}

func (s *initialImageStore) isReady() bool {
	if s.ready.Load() {
		return true
	}
	now := time.Now().Unix()
	if last := s.checkedAt.Load(); now == last || !s.checkedAt.CompareAndSwap(last, now) {
		return false
	}
	if _, err := os.Stat(filepath.Join(s.dir, initialImageDoneMarker)); err == nil {
		s.ready.Store(true)
		return true
	}
	return false
}

// read は、書き出した初期データの画像を読む。無ければ false を返し、呼び出し側は DB から読む
func (s *initialImageStore) read(id string) ([]byte, bool) {
	if !imageIDPattern.MatchString(id) || !s.isReady() {
		return nil, false
	}
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			log.Printf("failed to read initial image %s: %v", id, err)
		}
		return nil, false
	}
	return data, true
}

// readMany は、エクスポートの 1 人分（最大で数百枚）を並べて読む。冷えたディスクで 1 枚ずつ待たないため
func (s *initialImageStore) readMany(ids []string) map[string][]byte {
	found := make(map[string][]byte, len(ids))
	if len(ids) == 0 || !s.isReady() {
		return found
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for _, id := range ids {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			if data, ok := s.read(id); ok {
				mu.Lock()
				found[id] = data
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	return found
}

// ExtractInitialImages は、初期データの投稿の画像を dir に書き出す（ベンチの外で一度だけ流す）。
// 何度流しても壊れない：書き出し済みのファイルは読み直さず、途中で止めても続きから書く
func ExtractInitialImages(ctx context.Context, config *Config) error {
	db, err := sqlx.Open("pgx", config.DatabaseDSN())
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(2)
	return initialImages.extract(ctx, db)
}

func (s *initialImageStore) extract(ctx context.Context, db *sqlx.DB) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(s.dir, initialImageDoneMarker))

	var posts []struct {
		ImageID  string    `db:"image_id"`
		PostedAt time.Time `db:"posted_at"`
	}
	// 投稿の画像は posted_at の区画に書いてあるので、(id, 時刻) で主キーから 1 行ずつ引ける
	if err := db.SelectContext(ctx, &posts, `
  SELECT image_id, posted_at FROM posts
  WHERE posted_at <= (SELECT max(created_at) FROM initial_data_boundary)
  ORDER BY posted_at
 `); err != nil {
		return err
	}
	var todoIDs []string
	var todoTimes []time.Time
	for _, post := range posts {
		if !imageIDPattern.MatchString(post.ImageID) {
			continue
		}
		if _, err := os.Stat(s.path(post.ImageID)); err == nil {
			continue
		}
		todoIDs = append(todoIDs, post.ImageID)
		todoTimes = append(todoTimes, post.PostedAt)
	}
	log.Printf("initial images: %d posts, %d to write", len(posts), len(todoIDs))

	const batch = 500
	written := 0
	for start := 0; start < len(todoIDs); start += batch {
		end := min(start+batch, len(todoIDs))
		var rows []struct {
			ID   string `db:"id"`
			Data []byte `db:"data"`
		}
		if err := db.SelectContext(ctx, &rows, `
   SELECT i.id, i.data FROM unnest($1::text[], $2::timestamptz[]) AS v(id, created_at)
   JOIN image_objects i ON i.id = v.id AND i.created_at = v.created_at
  `, todoIDs[start:end], todoTimes[start:end]); err != nil {
			return err
		}
		for _, row := range rows {
			if err := s.writeFile(row.ID, row.Data); err != nil {
				return err
			}
		}
		written += len(rows)
		if (start/batch)%40 == 0 {
			log.Printf("initial images: %d/%d written", written, len(todoIDs))
		}
	}
	// ファイルの中身をディスクに確定させてから印を置く（電源が落ちても、空のファイルを画像として返さない）
	syscall.Sync()
	marker, err := os.Create(filepath.Join(s.dir, initialImageDoneMarker))
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(marker, "%d images\n", len(posts)); err != nil {
		marker.Close()
		return err
	}
	if err := marker.Sync(); err != nil {
		marker.Close()
		return err
	}
	if err := marker.Close(); err != nil {
		return err
	}
	syscall.Sync()
	log.Printf("initial images: done (%d written now, %d in total)", written, len(posts))
	return nil
}

func (s *initialImageStore) writeFile(id string, data []byte) error {
	dir := filepath.Dir(s.path(id))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path(id))
}
