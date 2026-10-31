package isureal

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"testing"
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
