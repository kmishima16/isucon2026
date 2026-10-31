package isureal

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// asia1 は SHA の専用命令の無い c5 で、CPU の 6 割近くを画像 URL の署名に使っていた。
// 専用命令のある europe1・america1 で計算できるよう、全台が VPC の中だけに署名の窓口を開ける（nginx は :8081 を通さない）
const maxSignItems = 2000
const maxSignRequestBytes = 1 << 20

type signRequestItem struct {
	Key string `json:"key"`
	ID  string `json:"id"`
}

type signRequest struct {
	Exp   int64             `json:"exp"`
	Items []signRequestItem `json:"items"`
}

type signResponse struct {
	Sigs []string `json:"sigs"`
}

var privateNetwork = &net.IPNet{IP: net.IPv4(192, 168, 0, 0), Mask: net.CIDRMask(16, 32)}

func fromPrivateNetwork(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || privateNetwork.Contains(ip))
}

func NewSignerHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /internal/sign", func(w http.ResponseWriter, r *http.Request) {
		if !fromPrivateNetwork(r.RemoteAddr) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var req signRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSignRequestBytes)).Decode(&req); err != nil || len(req.Items) == 0 || len(req.Items) > maxSignItems {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		keys := make([][]byte, len(req.Items))
		for i, item := range req.Items {
			key, err := hex.DecodeString(item.Key)
			if err != nil || len(key) != sha256.Size {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			keys[i] = key
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(signResponse{Sigs: signAll(keys, req.Items, req.Exp)})
	})
	registerImageStoreHandlers(mux)
	registerWindowPushHandler(mux)
	return mux
}

func signAll(keys [][]byte, items []signRequestItem, exp int64) []string {
	sigs := make([]string, len(items))
	var next atomic.Int64
	var wg sync.WaitGroup
	for range runtime.NumCPU() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1) - 1)
				if i >= len(items) {
					return
				}
				sigs[i] = base64.RawURLEncoding.EncodeToString(imageSignature(keys[i], items[i].ID, exp))
			}
		}()
	}
	wg.Wait()
	return sigs
}

// asia1 だけが europe1・america1 に頼む。IP は CloudFormation で決まっている。ほかのリージョンは自分で計算する
func signerPeersForRegion(region string) []string {
	if region == "asia" {
		return []string{"http://192.168.2.10:8081", "http://192.168.3.10:8081"}
	}
	return nil
}

type remoteSigner struct {
	peers    []string
	client   *http.Client
	timeout  time.Duration
	minBatch int
}

// nil なら頼まない。NewServer で決める
var imageSigner *remoteSigner

func newRemoteSigner(peers []string) *remoteSigner {
	if len(peers) == 0 {
		return nil
	}
	// 往復 100ms の台に頼むので、待ちは 300ms まで。数件なら往復のほうが高くつくので自分で計算する
	return &remoteSigner{
		peers:    peers,
		client:   &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: 32, IdleConnTimeout: 90 * time.Second}},
		timeout:  300 * time.Millisecond,
		minBatch: 8,
	}
}

func appendProfileImageJob(jobs []signJob, user User) []signJob {
	if user.ProfileImageID == nil {
		return jobs
	}
	return append(jobs, signJob{Key: user.ImageKey, ImageID: *user.ProfileImageID})
}

// 応答を組み立てる前に呼ぶ。届かなかった分は、組み立てのときに今までどおりこの台で計算する
func prefetchImageSignatures(ctx context.Context, jobs []signJob) {
	// europe1・america1 から転送されたリクエストで頼むと、頼み先が転送元の遠い台になり、往復が 1 回増える。件数も少ないので自分で計算する
	if imageSigner == nil || isProxiedRequest(ctx) {
		return
	}
	imageSigner.prefetch(ctx, signedURLCache, jobs, imageURLExp(time.Now()))
}

func (s *remoteSigner) prefetch(ctx context.Context, cache *signatureCache, jobs []signJob, exp int64) {
	seen := make(map[string]bool, len(jobs))
	missing := make([]signJob, 0, len(jobs))
	for _, job := range jobs {
		k := string(job.Key) + job.ImageID
		if seen[k] {
			continue
		}
		seen[k] = true
		if _, ok := cache.peek(job.Key, job.ImageID, exp); ok {
			continue
		}
		missing = append(missing, job)
	}
	if len(missing) < s.minBatch {
		return
	}
	if limit := maxSignItems * len(s.peers); len(missing) > limit {
		missing = missing[:limit]
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	size := (len(missing) + len(s.peers) - 1) / len(s.peers)
	var wg sync.WaitGroup
	for i, peer := range s.peers {
		lo := i * size
		if lo >= len(missing) {
			break
		}
		hi := min(lo+size, len(missing))
		chunk := missing[lo:hi]
		wg.Add(1)
		go func() {
			defer wg.Done()
			sigs, err := s.request(ctx, peer, chunk, exp)
			if err != nil {
				return
			}
			cache.putMany(chunk, sigs, exp)
		}()
	}
	wg.Wait()
}

func (s *remoteSigner) request(ctx context.Context, peer string, jobs []signJob, exp int64) ([]string, error) {
	req := signRequest{Exp: exp, Items: make([]signRequestItem, len(jobs))}
	for i, job := range jobs {
		req.Items[i] = signRequestItem{Key: hex.EncodeToString(job.Key), ID: job.ImageID}
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, peer+"/internal/sign", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("signer %s: status %d", peer, resp.StatusCode)
	}
	var out signResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxSignRequestBytes)).Decode(&out); err != nil {
		return nil, err
	}
	// 本文を最後まで読まないと接続が使い回されず、往復 100ms の台に毎回つなぎ直して待ちの 300ms を食う
	_, _ = io.Copy(io.Discard, resp.Body)
	if len(out.Sigs) != len(jobs) {
		return nil, fmt.Errorf("signer %s: %d signatures for %d items", peer, len(out.Sigs), len(jobs))
	}
	return out.Sigs, nil
}
