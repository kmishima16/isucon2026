package isureal

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const imageURLLifetime = 24 * time.Hour
const imageSignatureIterations = 1000

func imageSigningKey(secretKeyHash string) ([]byte, error) {
	key, err := hex.DecodeString(secretKeyHash)
	if err != nil || len(key) != sha256.Size {
		return nil, fmt.Errorf("invalid image signing key")
	}
	return key, nil
}

func imageSignature(key []byte, imageID string, exp int64) []byte {
	// 1000 回のたびに鍵から HMAC を作り直すと、署名が CPU の大半を使う。Reset は鍵を入れた状態に戻すだけで、結果は同じ
	mac := hmac.New(sha256.New, key)
	value := []byte(strconv.FormatInt(exp, 10) + ":" + imageID)
	var sum [sha256.Size]byte
	for range imageSignatureIterations {
		mac.Reset()
		mac.Write(value)
		value = mac.Sum(sum[:0])
	}
	return bytes.Clone(value)
}

// exp は 1 時間単位なので、同じ 1 時間に同じ画像を返す応答（フィード・Memories・エクスポートなど）は署名を使い回せる
type signatureCache struct {
	mu   sync.Mutex
	exp  int64
	sigs map[string]string
}

const signatureCacheLimit = 500000

var signedURLCache = &signatureCache{}

// 応答が署名しようとしている画像。まとめて europe1・america1 に頼むときの単位（image_signer.go）
type signJob struct {
	Key     []byte
	ImageID string
}

// この台で署名を計算した回数。取り寄せた署名で計算が消えたかを、テストで確かめるため
var signatureComputations atomic.Int64

func (c *signatureCache) peek(key []byte, imageID string, exp int64) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.exp != exp {
		return "", false
	}
	sig, ok := c.sigs[string(key)+imageID]
	return sig, ok
}

// ほかの台で計算した署名を入れる。古い 1 時間の分が遅れて届いても、今の分を捨てないよう入れない
func (c *signatureCache) putMany(jobs []signJob, sigs []string, exp int64) {
	if len(jobs) != len(sigs) {
		return
	}
	raws := make([][]byte, len(sigs))
	for i, sig := range sigs {
		raw, err := base64.RawURLEncoding.DecodeString(sig)
		if err != nil || len(raw) != sha256.Size {
			return
		}
		raws[i] = raw
	}
	c.mu.Lock()
	if c.exp > exp {
		c.mu.Unlock()
		return
	}
	if c.exp != exp || len(c.sigs)+len(jobs) > signatureCacheLimit {
		c.exp = exp
		c.sigs = make(map[string]string)
	}
	for i, job := range jobs {
		// 画像の取得の検証は正規の表記しか通さないので、受け取った文字列ではなく、バイト列から作り直した表記を入れる
		c.sigs[string(job.Key)+job.ImageID] = base64.RawURLEncoding.EncodeToString(raws[i])
	}
	c.mu.Unlock()
	for i, job := range jobs {
		verifiedImageSignatures.add(verifiedSignatureKey(job.Key, job.ImageID, exp, raws[i]), exp)
	}
}

func (c *signatureCache) get(key []byte, imageID string, exp int64) string {
	cacheKey := string(key) + imageID
	c.mu.Lock()
	if c.exp == exp {
		if sig, ok := c.sigs[cacheKey]; ok {
			c.mu.Unlock()
			return sig
		}
	}
	c.mu.Unlock()
	signatureComputations.Add(1)
	raw := imageSignature(key, imageID, exp)
	// この台が出した URL はこの台に取りに来るので、検証でもう一度 1000 回計算しないよう、通る署名として覚えておく
	verifiedImageSignatures.add(verifiedSignatureKey(key, imageID, exp, raw), exp)
	sig := base64.RawURLEncoding.EncodeToString(raw)
	c.mu.Lock()
	// 1 時間分を持つと数十万件になりうるので、上限を超えたら捨てて作り直す
	if c.exp != exp || len(c.sigs) >= signatureCacheLimit {
		c.exp = exp
		c.sigs = make(map[string]string)
	}
	c.sigs[cacheKey] = sig
	c.mu.Unlock()
	return sig
}

// 同じ URL の画像を何度も取りに来るので、検証に通った署名とこの台で作った署名は期限まで覚えておく。通らなかったものは毎回計算する（guard の判定を変えない）
type verifiedSignatures struct {
	mu      sync.Mutex
	entries map[string]int64
}

const verifiedSignaturesLimit = 200000

var verifiedImageSignatures = &verifiedSignatures{entries: make(map[string]int64)}

func verifiedSignatureKey(key []byte, imageID string, exp int64, sig []byte) string {
	return string(key) + imageID + ":" + strconv.FormatInt(exp, 10) + ":" + string(sig)
}

func (c *verifiedSignatures) contains(cacheKey string) bool {
	c.mu.Lock()
	_, ok := c.entries[cacheKey]
	c.mu.Unlock()
	return ok
}

func (c *verifiedSignatures) add(cacheKey string, exp int64) {
	c.mu.Lock()
	if len(c.entries) >= verifiedSignaturesLimit {
		now := time.Now().Unix()
		for k, e := range c.entries {
			if e <= now {
				delete(c.entries, k)
			}
		}
		if len(c.entries) >= verifiedSignaturesLimit {
			c.entries = make(map[string]int64)
		}
	}
	c.entries[cacheKey] = exp
	c.mu.Unlock()
}

func verifyImageSignature(key []byte, imageID string, exp int64, sig []byte) bool {
	cacheKey := verifiedSignatureKey(key, imageID, exp, sig)
	if verifiedImageSignatures.contains(cacheKey) {
		return true
	}
	if !hmac.Equal(sig, imageSignature(key, imageID, exp)) {
		return false
	}
	verifiedImageSignatures.add(cacheKey, exp)
	return true
}

// exp を秒のままにすると、秒が変わるたびにすべての画像の署名を作り直すことになり、asia1 の CPU の約 3 割を使っていた。
// 60 秒単位にしても、Memories とエクスポートは人ごとに違う画像を返すので当たりにくく、40100 点の回でも asia1 の CPU の 57% が署名だった。
// 1 時間単位に切り下げて、同じ 1 時間の中は署名を使い回す（人間が判断）。有効時間が最大 59 分短くなるだけで、形式と検証は変わらない
const imageURLExpStep = 3600

func imageURLExp(now time.Time) int64 {
	exp := now.Add(imageURLLifetime).Unix()
	return exp - exp%imageURLExpStep
}

func signedImageURL(user User, imageID string) string {
	exp := imageURLExp(time.Now())
	query := url.Values{
		"exp": {strconv.FormatInt(exp, 10)},
		"sig": {signedURLCache.get(user.ImageKey, imageID, exp)},
	}
	return "/api/images/" + url.PathEscape(imageID) + "?" + query.Encode()
}
