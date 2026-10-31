package isureal

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// 初期実装の署名。速くした実装が同じ値を返すことを確かめるために残す
func referenceImageSignature(key []byte, imageID string, exp int64) []byte {
	value := []byte(strconv.FormatInt(exp, 10) + ":" + imageID)
	for range imageSignatureIterations {
		mac := hmac.New(sha256.New, key)
		mac.Write(value)
		value = mac.Sum(nil)
	}
	return value
}

var testSigningKey, _ = imageSigningKey("8f434346648f6b96df89dda901c5176b10a6d83961dd3c1ac88b59b2dc327aa4")

func TestImageSignatureMatchesReference(t *testing.T) {
	cases := []struct {
		imageID string
		exp     int64
	}{
		{"777d5e88-9eb9-4084-ac7d-b4c3743ff421", 1793523600},
		{"c55745fb-68eb-481d-8ecb-5cc8053628d7", 1},
		{"", 0},
		{"long-" + string(make([]byte, 200)), 9999999999},
	}
	for _, tc := range cases {
		got := imageSignature(testSigningKey, tc.imageID, tc.exp)
		want := referenceImageSignature(testSigningKey, tc.imageID, tc.exp)
		if !hmac.Equal(got, want) {
			t.Fatalf("signature mismatch for %q %d", tc.imageID, tc.exp)
		}
	}
}

func TestSignedURLCacheSeparatesKeysAndExp(t *testing.T) {
	otherKey, _ := imageSigningKey("0000000000000000000000000000000000000000000000000000000000000001")
	c := &signatureCache{}
	id := "777d5e88-9eb9-4084-ac7d-b4c3743ff421"
	for _, tc := range []struct {
		key []byte
		exp int64
	}{{testSigningKey, 100}, {otherKey, 100}, {testSigningKey, 100}, {testSigningKey, 101}, {otherKey, 101}} {
		got := c.get(tc.key, id, tc.exp)
		want := base64.RawURLEncoding.EncodeToString(referenceImageSignature(tc.key, id, tc.exp))
		if got != want {
			t.Fatalf("cached signature mismatch for exp %d", tc.exp)
		}
	}
}

func TestVerifyImageSignature(t *testing.T) {
	id := "777d5e88-9eb9-4084-ac7d-b4c3743ff421"
	exp := int64(1793523600)
	good := referenceImageSignature(testSigningKey, id, exp)
	for range 2 {
		if !verifyImageSignature(testSigningKey, id, exp, good) {
			t.Fatal("valid signature rejected")
		}
	}
	bad := append([]byte(nil), good...)
	bad[0] ^= 1
	if verifyImageSignature(testSigningKey, id, exp, bad) {
		t.Fatal("invalid signature accepted")
	}
	if verifyImageSignature(testSigningKey, id, exp+1, good) {
		t.Fatal("signature accepted for another exp")
	}
}

func TestSignedSignatureIsRememberedForVerify(t *testing.T) {
	id := "c55745fb-68eb-481d-8ecb-5cc8053628d7"
	exp := int64(1793523660)
	sig, err := base64.RawURLEncoding.DecodeString((&signatureCache{}).get(testSigningKey, id, exp))
	if err != nil {
		t.Fatal(err)
	}
	if !verifiedImageSignatures.contains(verifiedSignatureKey(testSigningKey, id, exp, sig)) {
		t.Fatal("signature made here is not remembered for verify")
	}
	if !verifyImageSignature(testSigningKey, id, exp, sig) {
		t.Fatal("signature made here does not verify")
	}
}

func TestImageURLExpRoundsDownToHour(t *testing.T) {
	hour := int64(1793523600)
	for _, tc := range []struct {
		now  int64
		want int64
	}{
		{hour, hour + 86400},
		{hour + 59, hour + 86400},
		{hour + 3599, hour + 86400},
		{hour + 3600, hour + 90000},
	} {
		if got := imageURLExp(time.Unix(tc.now, 0)); got != tc.want {
			t.Fatalf("imageURLExp(%d) = %d, want %d", tc.now, got, tc.want)
		}
	}
}

func TestSignedImageURLVerifiesWithRoundedExp(t *testing.T) {
	id := "777d5e88-9eb9-4084-ac7d-b4c3743ff421"
	before := time.Now()
	raw := signedImageURL(User{ImageKey: testSigningKey}, id)
	u, err := url.Parse(raw)
	if err != nil || u.Path != "/api/images/"+id {
		t.Fatalf("unexpected url %q", raw)
	}
	exp, err := strconv.ParseInt(u.Query().Get("exp"), 10, 64)
	if err != nil || exp%imageURLExpStep != 0 || exp > before.Add(imageURLLifetime).Unix()+imageURLExpStep || exp <= before.Add(imageURLLifetime).Unix()-imageURLExpStep {
		t.Fatalf("unexpected exp %q", u.Query().Get("exp"))
	}
	sig, err := base64.RawURLEncoding.DecodeString(u.Query().Get("sig"))
	if err != nil || !verifyImageSignature(testSigningKey, id, exp, sig) {
		t.Fatal("signed url does not verify")
	}
}

func TestSignatureCachePeekAndPutMany(t *testing.T) {
	c := &signatureCache{}
	id := "777d5e88-9eb9-4084-ac7d-b4c3743ff421"
	exp := int64(1793523600)
	if _, ok := c.peek(testSigningKey, id, exp); ok {
		t.Fatal("empty cache returned a signature")
	}
	sig := base64.RawURLEncoding.EncodeToString(referenceImageSignature(testSigningKey, id, exp))
	c.putMany([]signJob{{Key: testSigningKey, ImageID: id}}, []string{sig}, exp)
	got, ok := c.peek(testSigningKey, id, exp)
	if !ok || got != sig {
		t.Fatal("putMany did not store the signature")
	}
	before := signatureComputations.Load()
	if c.get(testSigningKey, id, exp) != sig {
		t.Fatal("get returned another signature")
	}
	if signatureComputations.Load() != before {
		t.Fatal("get recomputed a signature that was put")
	}
	raw, _ := base64.RawURLEncoding.DecodeString(sig)
	if !verifiedImageSignatures.contains(verifiedSignatureKey(testSigningKey, id, exp, raw)) {
		t.Fatal("put signature is not remembered for verify")
	}
}

func TestSignatureCacheGetCountsComputations(t *testing.T) {
	c := &signatureCache{}
	before := signatureComputations.Load()
	c.get(testSigningKey, "count-me", 1793523600)
	c.get(testSigningKey, "count-me", 1793523600)
	if got := signatureComputations.Load() - before; got != 1 {
		t.Fatalf("computations = %d, want 1", got)
	}
}

func TestSignatureCachePutManyIgnoresOlderExpAndBadInput(t *testing.T) {
	c := &signatureCache{}
	id := "777d5e88-9eb9-4084-ac7d-b4c3743ff421"
	exp := int64(1793523600)
	sig := base64.RawURLEncoding.EncodeToString(referenceImageSignature(testSigningKey, id, exp))
	c.putMany([]signJob{{Key: testSigningKey, ImageID: id}}, []string{sig}, exp)

	older := exp - 3600
	oldSig := base64.RawURLEncoding.EncodeToString(referenceImageSignature(testSigningKey, "late", older))
	c.putMany([]signJob{{Key: testSigningKey, ImageID: "late"}}, []string{oldSig}, older)
	if _, ok := c.peek(testSigningKey, id, exp); !ok {
		t.Fatal("a late batch for an older hour reset the cache")
	}
	if _, ok := c.peek(testSigningKey, "late", older); ok {
		t.Fatal("a late batch for an older hour was stored")
	}

	c.putMany([]signJob{{Key: testSigningKey, ImageID: "bad"}}, []string{"not base64!"}, exp)
	if _, ok := c.peek(testSigningKey, "bad", exp); ok {
		t.Fatal("stored a malformed signature")
	}
	c.putMany([]signJob{{Key: testSigningKey, ImageID: "short"}}, []string{"AAAA"}, exp)
	if _, ok := c.peek(testSigningKey, "short", exp); ok {
		t.Fatal("stored a signature of the wrong length")
	}
	c.putMany([]signJob{{Key: testSigningKey, ImageID: "y"}, {Key: testSigningKey, ImageID: "z"}}, []string{sig}, exp)
	if _, ok := c.peek(testSigningKey, "y", exp); ok {
		t.Fatal("stored signatures when the counts differ")
	}

	newer := exp + 3600
	newSig := base64.RawURLEncoding.EncodeToString(referenceImageSignature(testSigningKey, id, newer))
	c.putMany([]signJob{{Key: testSigningKey, ImageID: id}}, []string{newSig}, newer)
	if got, ok := c.peek(testSigningKey, id, newer); !ok || got != newSig {
		t.Fatal("a batch for a newer hour was not stored")
	}
}

func TestSignatureCachePutManyStoresCanonicalEncoding(t *testing.T) {
	c := &signatureCache{}
	id := "777d5e88-9eb9-4084-ac7d-b4c3743ff421"
	exp := int64(1793523600)
	canonical := base64.RawURLEncoding.EncodeToString(referenceImageSignature(testSigningKey, id, exp))
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	last := strings.IndexByte(alphabet, canonical[len(canonical)-1])
	// 43 文字目の下位 2 ビットは使われない。立てても寛容な復号では同じバイト列になるが、画像の取得の検証は正規の表記以外を拒む
	loose := canonical[:len(canonical)-1] + string(alphabet[last|1])
	if loose == canonical {
		loose = canonical[:len(canonical)-1] + string(alphabet[last|2])
	}
	c.putMany([]signJob{{Key: testSigningKey, ImageID: id}}, []string{loose}, exp)
	if got, ok := c.peek(testSigningKey, id, exp); ok && got != canonical {
		t.Fatalf("stored a non-canonical signature %q, want %q or nothing", got, canonical)
	}
}

func BenchmarkImageSignature(b *testing.B) {
	for b.Loop() {
		imageSignature(testSigningKey, "777d5e88-9eb9-4084-ac7d-b4c3743ff421", 1793523600)
	}
}

func BenchmarkReferenceImageSignature(b *testing.B) {
	for b.Loop() {
		referenceImageSignature(testSigningKey, "777d5e88-9eb9-4084-ac7d-b4c3743ff421", 1793523600)
	}
}
