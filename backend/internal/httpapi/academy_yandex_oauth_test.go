package httpapi

import (
	"strings"
	"testing"
	"time"
)

func TestCloudOAuthSignedStateRejectsTamperingAndExpiry(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	state := cloudOAuthState{
		Actor:    "author-one",
		Verifier: strings.Repeat("v", 43),
		Nonce:    "random-nonce",
		Expires:  time.Now().Add(time.Minute).Unix(),
	}
	sealed, err := cloudSeal(state, key)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := cloudOpen(sealed, key)
	if err != nil || opened.Actor != state.Actor || opened.Nonce != state.Nonce {
		t.Fatalf("state round trip: %+v, %v", opened, err)
	}
	if _, err := cloudOpen(sealed, []byte("different-secret-key-012345678901")); err == nil {
		t.Fatal("foreign key accepted")
	}
	parts := strings.Split(sealed, ".")
	if len(parts) != 2 {
		t.Fatal("missing state signature")
	}
	parts[1] = strings.Repeat("A", len(parts[1]))
	if _, err := cloudOpen(strings.Join(parts, "."), key); err == nil {
		t.Fatal("tampered signature accepted")
	}
	state.Expires = time.Now().Add(-time.Minute).Unix()
	expired, err := cloudSeal(state, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cloudOpen(expired, key); err == nil {
		t.Fatal("expired OAuth flow accepted")
	}
}
