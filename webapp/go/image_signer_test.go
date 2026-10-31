package isureal

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func postSign(remoteAddr, method, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/internal/sign", strings.NewReader(body))
	req.RemoteAddr = remoteAddr
	rec := httptest.NewRecorder()
	NewSignerHandler().ServeHTTP(rec, req)
	return rec
}

func TestSignerHandlerMatchesImageSignature(t *testing.T) {
	exp := int64(1793523600)
	otherKey, err := imageSigningKey(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	keys := [][]byte{testSigningKey, otherKey, testSigningKey}
	ids := []string{"777d5e88-9eb9-4084-ac7d-b4c3743ff421", "c55745fb-68eb-481d-8ecb-5cc8053628d7", "img-3"}
	req := signRequest{Exp: exp}
	for i := range ids {
		req.Items = append(req.Items, signRequestItem{Key: hex.EncodeToString(keys[i]), ID: ids[i]})
	}
	body, _ := json.Marshal(req)
	rec := postSign("192.168.1.10:40000", http.MethodPost, string(body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp signResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Sigs) != len(ids) {
		t.Fatalf("got %d signatures, want %d", len(resp.Sigs), len(ids))
	}
	for i := range ids {
		want := base64.RawURLEncoding.EncodeToString(referenceImageSignature(keys[i], ids[i], exp))
		if resp.Sigs[i] != want {
			t.Fatalf("signature %d differs from imageSignature", i)
		}
	}
}

func TestSignerHandlerRejects(t *testing.T) {
	key := strings.Repeat("ab", 32)
	good := `{"exp":1,"items":[{"key":"` + key + `","id":"x"}]}`
	var many strings.Builder
	many.WriteString(`{"exp":1,"items":[`)
	for i := range maxSignItems + 1 {
		if i > 0 {
			many.WriteString(",")
		}
		many.WriteString(`{"key":"` + key + `","id":"x"}`)
	}
	many.WriteString(`]}`)
	cases := []struct {
		name, remote, method, body string
		want                       int
	}{
		{"vpc", "192.168.2.10:1", http.MethodPost, good, http.StatusOK},
		{"loopback", "127.0.0.1:1", http.MethodPost, good, http.StatusOK},
		{"outside vpc", "203.0.113.5:1", http.MethodPost, good, http.StatusForbidden},
		{"router public side", "10.0.0.5:1", http.MethodPost, good, http.StatusForbidden},
		{"get", "192.168.2.10:1", http.MethodGet, "", http.StatusMethodNotAllowed},
		{"bad hex key", "192.168.2.10:1", http.MethodPost, `{"exp":1,"items":[{"key":"zz","id":"x"}]}`, http.StatusBadRequest},
		{"short key", "192.168.2.10:1", http.MethodPost, `{"exp":1,"items":[{"key":"abcd","id":"x"}]}`, http.StatusBadRequest},
		{"no items", "192.168.2.10:1", http.MethodPost, `{"exp":1,"items":[]}`, http.StatusBadRequest},
		{"not json", "192.168.2.10:1", http.MethodPost, `{`, http.StatusBadRequest},
		{"too many items", "192.168.2.10:1", http.MethodPost, many.String(), http.StatusBadRequest},
	}
	for _, tc := range cases {
		if rec := postSign(tc.remote, tc.method, tc.body); rec.Code != tc.want {
			t.Errorf("%s: status %d, want %d", tc.name, rec.Code, tc.want)
		}
	}
}

func newTestSignerServer(t *testing.T, behave func(w http.ResponseWriter, r *http.Request) bool) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var calls atomic.Int64
	inner := NewSignerHandler()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if behave != nil && behave(w, r) {
			return
		}
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func testSignJobs(n int) []signJob {
	jobs := make([]signJob, 0, n)
	for i := range n {
		jobs = append(jobs, signJob{Key: testSigningKey, ImageID: fmt.Sprintf("img-%03d", i)})
	}
	return jobs
}

func TestRemoteSignerPrefetchFillsCache(t *testing.T) {
	s1, c1 := newTestSignerServer(t, nil)
	s2, c2 := newTestSignerServer(t, nil)
	signer := &remoteSigner{peers: []string{s1.URL, s2.URL}, client: &http.Client{}, timeout: 5 * time.Second, minBatch: 1}
	cache := &signatureCache{}
	exp := int64(1793523600)
	signer.prefetch(context.Background(), cache, testSignJobs(10), exp)
	if c1.Load() != 1 || c2.Load() != 1 {
		t.Fatalf("calls = %d, %d; want 1 each", c1.Load(), c2.Load())
	}
	for _, job := range testSignJobs(10) {
		got, ok := cache.peek(job.Key, job.ImageID, exp)
		want := base64.RawURLEncoding.EncodeToString(referenceImageSignature(job.Key, job.ImageID, exp))
		if !ok || got != want {
			t.Fatalf("%s was not prefetched correctly", job.ImageID)
		}
	}
}

func TestRemoteSignerPrefetchDedupesAndSkipsCached(t *testing.T) {
	var sent atomic.Int64
	srv, _ := newTestSignerServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		var req signRequest
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		sent.Store(int64(len(req.Items)))
		r.Body = io.NopCloser(bytes.NewReader(body))
		return false
	})
	signer := &remoteSigner{peers: []string{srv.URL}, client: &http.Client{}, timeout: 5 * time.Second, minBatch: 1}
	cache := &signatureCache{}
	exp := int64(1793523600)
	cached := testSignJobs(3)
	sigs := make([]string, len(cached))
	for i, job := range cached {
		sigs[i] = base64.RawURLEncoding.EncodeToString(referenceImageSignature(job.Key, job.ImageID, exp))
	}
	cache.putMany(cached, sigs, exp)
	jobs := append(testSignJobs(10), testSignJobs(10)...)
	signer.prefetch(context.Background(), cache, jobs, exp)
	if sent.Load() != 7 {
		t.Fatalf("sent %d items, want 7 (10 unique minus 3 cached)", sent.Load())
	}
}

func TestRemoteSignerPrefetchSkipsSmallBatches(t *testing.T) {
	srv, calls := newTestSignerServer(t, nil)
	signer := &remoteSigner{peers: []string{srv.URL}, client: &http.Client{}, timeout: 5 * time.Second, minBatch: 8}
	signer.prefetch(context.Background(), &signatureCache{}, testSignJobs(7), 1793523600)
	if calls.Load() != 0 {
		t.Fatalf("sent a batch of 7 although minBatch is 8")
	}
}

func TestRemoteSignerPrefetchFallsBack(t *testing.T) {
	broken, _ := newTestSignerServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		http.Error(w, "boom", http.StatusInternalServerError)
		return true
	})
	slow, _ := newTestSignerServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		time.Sleep(500 * time.Millisecond)
		return false
	})
	short, _ := newTestSignerServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		_, _ = w.Write([]byte(`{"sigs":["AAAA"]}`))
		return true
	})
	exp := int64(1793523600)
	for _, peer := range []string{broken.URL, slow.URL, short.URL} {
		signer := &remoteSigner{peers: []string{peer}, client: &http.Client{}, timeout: 100 * time.Millisecond, minBatch: 1}
		cache := &signatureCache{}
		start := time.Now()
		signer.prefetch(context.Background(), cache, testSignJobs(4), exp)
		if time.Since(start) > 400*time.Millisecond {
			t.Fatalf("%s: prefetch waited %v, longer than the timeout", peer, time.Since(start))
		}
		for _, job := range testSignJobs(4) {
			if _, ok := cache.peek(job.Key, job.ImageID, exp); ok {
				t.Fatalf("%s: stored a signature from a failed peer", peer)
			}
		}
		before := signatureComputations.Load()
		want := base64.RawURLEncoding.EncodeToString(referenceImageSignature(testSigningKey, "img-000", exp))
		if got := cache.get(testSigningKey, "img-000", exp); got != want {
			t.Fatalf("%s: local fallback returned another signature", peer)
		}
		if signatureComputations.Load() != before+1 {
			t.Fatalf("%s: fallback did not compute locally", peer)
		}
	}
}

func TestSignerPeersForRegion(t *testing.T) {
	asia := signerPeersForRegion("asia")
	if len(asia) != 2 || asia[0] != "http://192.168.2.10:8081" || asia[1] != "http://192.168.3.10:8081" {
		t.Fatalf("asia peers = %v", asia)
	}
	for _, region := range []string{"europe", "america"} {
		if peers := signerPeersForRegion(region); len(peers) != 0 {
			t.Fatalf("%s should sign locally, got %v", region, peers)
		}
		if newRemoteSigner(signerPeersForRegion(region)) != nil {
			t.Fatalf("%s should not get a remote signer", region)
		}
	}
}

func TestAppendProfileImageJob(t *testing.T) {
	image := "pimg"
	jobs := appendProfileImageJob(nil, User{ImageKey: testSigningKey})
	jobs = appendProfileImageJob(jobs, User{ImageKey: testSigningKey, ProfileImageID: &image})
	if len(jobs) != 1 || jobs[0].ImageID != "pimg" {
		t.Fatalf("jobs = %v", jobs)
	}
}

func TestRemoteSignerReusesConnection(t *testing.T) {
	var newConns atomic.Int64
	srv := httptest.NewUnstartedServer(NewSignerHandler())
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			newConns.Add(1)
		}
	}
	srv.Start()
	t.Cleanup(srv.Close)
	signer := &remoteSigner{peers: []string{srv.URL}, client: &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: 4}}, timeout: 5 * time.Second, minBatch: 1}
	for round := range 3 {
		jobs := make([]signJob, 0, 1000)
		for i := range 1000 {
			jobs = append(jobs, signJob{Key: testSigningKey, ImageID: fmt.Sprintf("r%d-img-%04d", round, i)})
		}
		signer.prefetch(context.Background(), &signatureCache{}, jobs, 1793523600)
	}
	if n := newConns.Load(); n != 1 {
		t.Fatalf("opened %d connections for 3 batches, want 1 (the response body must be read to the end)", n)
	}
}

func TestPrefetchSkipsProxiedRequests(t *testing.T) {
	srv, calls := newTestSignerServer(t, nil)
	saved := imageSigner
	imageSigner = &remoteSigner{peers: []string{srv.URL}, client: &http.Client{}, timeout: 5 * time.Second, minBatch: 1}
	t.Cleanup(func() { imageSigner = saved })
	jobs := func(prefix string) []signJob {
		out := make([]signJob, 0, 10)
		for i := range 10 {
			out = append(out, signJob{Key: testSigningKey, ImageID: fmt.Sprintf("%s-%03d", prefix, i)})
		}
		return out
	}

	proxied := context.WithValue(context.Background(), proxiedRegionKey{}, "europe")
	prefetchImageSignatures(proxied, jobs("proxied"))
	if calls.Load() != 0 {
		t.Fatalf("proxied request asked peers %d times; want 0", calls.Load())
	}
	prefetchImageSignatures(context.Background(), jobs("direct"))
	if calls.Load() != 1 {
		t.Fatalf("direct request asked peers %d times; want 1", calls.Load())
	}
}
