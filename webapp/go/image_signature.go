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

// exp は秒単位なので、同じ秒に同じ画像を返す応答（フィードの投稿者のプロフィール画像など）は署名を使い回せる
type signatureCache struct {
	mu   sync.Mutex
	exp  int64
	sigs map[string]string
}

var signedURLCache = &signatureCache{}

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
	sig := base64.RawURLEncoding.EncodeToString(imageSignature(key, imageID, exp))
	c.mu.Lock()
	if c.exp != exp {
		c.exp = exp
		c.sigs = make(map[string]string)
	}
	c.sigs[cacheKey] = sig
	c.mu.Unlock()
	return sig
}

// 同じ URL の画像を何度も取りに来るので、検証に通った署名は期限まで覚えておく。通らなかったものは毎回計算する（guard の判定を変えない）
type verifiedSignatures struct {
	mu      sync.Mutex
	entries map[string]int64
}

const verifiedSignaturesLimit = 200000

var verifiedImageSignatures = &verifiedSignatures{entries: make(map[string]int64)}

func verifyImageSignature(key []byte, imageID string, exp int64, sig []byte) bool {
	cacheKey := string(key) + imageID + ":" + strconv.FormatInt(exp, 10) + ":" + string(sig)
	c := verifiedImageSignatures
	c.mu.Lock()
	_, ok := c.entries[cacheKey]
	c.mu.Unlock()
	if ok {
		return true
	}
	if !hmac.Equal(sig, imageSignature(key, imageID, exp)) {
		return false
	}
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
	return true
}

func signedImageURL(user User, imageID string) string {
	exp := time.Now().Add(imageURLLifetime).Unix()
	query := url.Values{
		"exp": {strconv.FormatInt(exp, 10)},
		"sig": {signedURLCache.get(user.ImageKey, imageID, exp)},
	}
	return "/api/images/" + url.PathEscape(imageID) + "?" + query.Encode()
}
