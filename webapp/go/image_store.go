package isureal

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

// 投稿の画像の中身は、DB ではなく投稿を受けた台のディスクに置く。DB の時間の 1 位が、約 380KB の画像を TOAST に書く INSERT だったため。
// DB には中身を空にした行と、中身を持つ台（image_object_locations.host）を書く。見せてよいかの判断（署名・削除・退会）は今までどおり DB で行う。
// ほかの台の画像は、その台の内部の窓口（:8081、VPC の中からだけ届く）から取り寄せる。設計は logs/notes/image-files-design.md

var imageStoreDir = envOrDefault("ISUREAL_IMAGE_STORE", "/home/isucon/image-store")

var imageIDPattern = regexp.MustCompile(`^[0-9a-f-]{36}$`)

// 各台の内部の窓口。台の private IP は CloudFormation で決まっている。リージョンの既定の台（ISUREAL_IMAGE_SELF が無いときの自分）
var regionImageHosts = map[string]string{
	"asia":    "http://192.168.1.10:8081",
	"europe":  "http://192.168.2.10:8081",
	"america": "http://192.168.3.10:8081",
}

// アプリが動く全台の内部の窓口と、そのリージョン。asia2 は DB の台だが、asia1 の nginx から振り分けを受けてアプリも動かす（同じリージョンで往復 0.4ms）
var appImageHosts = map[string]string{
	"http://192.168.1.10:8081": "asia",
	"http://192.168.4.10:8081": "asia",
	"http://192.168.2.10:8081": "europe",
	"http://192.168.3.10:8081": "america",
}

// 既定の Transport は台ごとに 2 本しか接続を残さず、混むとリージョンをまたいで毎回つなぎ直して 2 秒を超えるので、署名と同じだけ残す
var imageHTTPClient = &http.Client{
	Timeout:   2 * time.Second,
	Transport: &http.Transport{MaxIdleConnsPerHost: 32, IdleConnTimeout: 90 * time.Second},
}

type imageStore struct {
	dir   string
	self  string
	peers []string
	// 同じリージョンでアプリが動くほかの台（asia1 と asia2）
	regionPeers []string
	cache       *ImageDataCache
}

var images = &imageStore{dir: imageStoreDir}

// configureImageStore は、この台の窓口とほかの台の窓口を決め、置き場所を作る
func configureImageStore(region string, cache *ImageDataCache) {
	images.self = envOrDefault("ISUREAL_IMAGE_SELF", regionImageHosts[region])
	images.peers = nil
	images.regionPeers = nil
	for host, hostRegion := range appImageHosts {
		if host == images.self {
			continue
		}
		images.peers = append(images.peers, host)
		if hostRegion == region {
			images.regionPeers = append(images.regionPeers, host)
		}
	}
	images.cache = cache
	if err := os.MkdirAll(images.dir, 0o755); err != nil {
		log.Printf("failed to create image store %s: %v", images.dir, err)
	}
}

func (s *imageStore) path(id string) string { return filepath.Join(s.dir, id) }

// write は、DB に書く前に中身をディスクに確定させる（再起動の後も残す）
func (s *imageStore) write(id string, data []byte) error {
	if !imageIDPattern.MatchString(id) {
		return fmt.Errorf("invalid image id %q", id)
	}
	tmp, err := os.CreateTemp(s.dir, ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), s.path(id)); err != nil {
		return err
	}
	dir, err := os.Open(s.dir)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func (s *imageStore) remove(id string) { _ = os.Remove(s.path(id)) }

func (s *imageStore) readLocal(id string) ([]byte, error) {
	if !imageIDPattern.MatchString(id) {
		return nil, fmt.Errorf("invalid image id %q", id)
	}
	return os.ReadFile(s.path(id))
}

// load は、中身を持つ台 host から画像を読む（この台なら自分のディスク、ほかの台なら窓口から）。読んだ中身は台ごとのメモリに入れる
func (s *imageStore) load(ctx context.Context, host, id string) ([]byte, error) {
	if data, ok := s.cache.get(id); ok {
		return data, nil
	}
	var data []byte
	var err error
	if host == s.self {
		data, err = s.readLocal(id)
	} else {
		data, err = s.fetch(ctx, host, id)
	}
	if err != nil {
		return nil, err
	}
	s.cache.set(id, data)
	return data, nil
}

func (s *imageStore) fetch(ctx context.Context, host, id string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, host+"/internal/images/"+id, nil)
	if err != nil {
		return nil, err
	}
	res, err := imageHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, res.Body)
		return nil, fmt.Errorf("fetch image %s from %s: %s", id, host, res.Status)
	}
	return io.ReadAll(res.Body)
}

// purgeLocal は、この台の置き場所とメモリを空にする（初期化）。置くのは走行中の投稿の画像だけで、初期化の後は DB から参照されない
func (s *imageStore) purgeLocal() {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		log.Printf("failed to read image store: %v", err)
	}
	for _, entry := range entries {
		_ = os.Remove(filepath.Join(s.dir, entry.Name()))
	}
	if s.cache != nil {
		s.cache.reset()
	}
}

// purgeAll は、この台とほかの台の置き場所を空にする。届かなかった台のファイルは残るが、DB に行が無いので返らない
func (s *imageStore) purgeAll(ctx context.Context) {
	s.purgeLocal()
	localPosts.reset()
	// 初期化で利用者は DB から消えるので、つながっている SSE に DB で確かめ直させる（接続のときの状態のまま通知しない）
	if hub := localWindowHub.Load(); hub != nil {
		hub.userChangedLocal("")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for _, peer := range s.peers {
		wg.Go(func() {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, peer+"/internal/purge-images", nil)
			if err != nil {
				return
			}
			res, err := imageHTTPClient.Do(req)
			if err != nil {
				log.Printf("failed to purge images on %s: %v", peer, err)
				return
			}
			_, _ = io.Copy(io.Discard, res.Body)
			res.Body.Close()
		})
	}
	wg.Wait()
}

// registerImageStoreHandlers は、内部の窓口（:8081）に画像を渡す口と空にする口を足す
func registerImageStoreHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /internal/images/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !fromPrivateNetwork(r.RemoteAddr) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		data, err := images.readLocal(r.PathValue("id"))
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(data)
	})
	mux.HandleFunc("POST /internal/user-changed", func(w http.ResponseWriter, r *http.Request) {
		if !fromPrivateNetwork(r.RemoteAddr) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		// 空なら、利用者を絞らない古い知らせ（全台を順に deploy する間に、古い版の台から届く）として全員に確かめ直させる
		userID := r.URL.Query().Get("user")
		if hub := localWindowHub.Load(); hub != nil {
			hub.userChangedLocal(userID)
		}
		if userID != "" {
			// ほかの台で退会やリージョンの変更を受けたとき、この台で覚えているセッションと今日の投稿は古くなる（session_cache.go・local_post_state.go）
			sessions.forgetUser(userID)
			localPosts.forget(userID)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /internal/purge-images", func(w http.ResponseWriter, r *http.Request) {
		if !fromPrivateNetwork(r.RemoteAddr) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		images.purgeLocal()
		// 初期化で投稿は DB から消えるので、この台で持っていた今日の投稿も捨てる（local_post_state.go）
		localPosts.reset()
		// 初期化でセッションは DB から消えるので、覚えたものも捨てる（session_cache.go）
		sessions.reset()
		// 初期化で利用者は DB から消えるので、つながっている SSE に DB で確かめ直させる（接続のときの状態のまま通知しない）
		if hub := localWindowHub.Load(); hub != nil {
			hub.userChangedLocal("")
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// notifyUserChanged は、ほかの全台に、その利用者の SSE の状態を確かめ直させる。届くのを待つのは 1 秒まで
func (s *imageStore) notifyUserChanged(userID string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for _, peer := range s.peers {
		wg.Go(func() {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, peer+"/internal/user-changed?user="+url.QueryEscape(userID), nil)
			if err != nil {
				return
			}
			res, err := imageHTTPClient.Do(req)
			if err != nil {
				log.Printf("failed to notify user change to %s: %v", peer, err)
				return
			}
			_, _ = io.Copy(io.Discard, res.Body)
			res.Body.Close()
		})
	}
	wg.Wait()
}
