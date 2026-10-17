package runtimepostgres

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestFinanceMFATOTPUsesPublishedVectorAndPreventsReplay(t *testing.T) {
	secret := []byte("12345678901234567890")
	now := time.Unix(59, 0)
	if got := financeMFACode(secret, 1); got != "287082" {
		t.Fatalf("RFC 6238 TOTP truncation invalid: %s", got)
	}
	if got := financeMFAMatch(secret, "287082", now, -1); got != 1 {
		t.Fatalf("actual factor not accepted at expected step: %d", got)
	}
	if got := financeMFAMatch(secret, "287082", now, 1); got != -1 {
		t.Fatalf("replayed code accepted: %d", got)
	}
	if got := financeMFAMatch(secret, "287082", now.Add(3*time.Minute), -1); got != -1 {
		t.Fatalf("expired code accepted: %d", got)
	}
	for _, invalid := range []string{"", "12345", "1234567", "abc123", " 287082", "287082 "} {
		if got := financeMFAMatch(secret, invalid, now, -1); got != -1 {
			t.Errorf("invalid code %q accepted at step %d", invalid, got)
		}
	}
}

func TestFinanceMFAAESGCMIdentityBindingAndTamper(t *testing.T) {
	var key FinanceMFAKey
	for i := range key {
		key[i] = byte(i + 1)
	}
	secret := bytes.Repeat([]byte{42}, 20)
	a := "11111111-1111-4111-8111-111111111111"
	encrypted, err := financeMFAEncrypt(key, a, secret)
	if err != nil {
		t.Fatal(err)
	}
	out, err := financeMFADecrypt(key, a, encrypted)
	if err != nil || !bytes.Equal(out, secret) {
		t.Fatal("factor decrypt failed", err)
	}
	b := "22222222-2222-4222-8222-222222222222"
	if _, err = financeMFADecrypt(key, b, encrypted); err == nil {
		t.Fatal("cross-identity decryption accepted")
	}
	encrypted[len(encrypted)-1] ^= 1
	if _, err = financeMFADecrypt(key, a, encrypted); err == nil {
		t.Fatal("tampered encrypted factor accepted")
	}
	if _, err = financeMFADecrypt(key, a, []byte(strings.Repeat("X", 16))); err == nil {
		t.Fatal("truncated factor accepted")
	}
}
