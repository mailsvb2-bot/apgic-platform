package growth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTokensAreTamperProofScopedAndExpire(t *testing.T) {
	now := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	if _, err := NewSigner([]byte("short")); !errors.Is(err, ErrSigningKey) {
		t.Fatal(err)
	}
	s, err := NewSigner([]byte(strings.Repeat("a", 32)))
	if err != nil {
		t.Fatal(err)
	}
	token, err := s.Issue("11111111-1111-4111-8111-111111111111", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.Verify(token, now)
	if err != nil || id != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("valid token id=%s err=%v", id, err)
	}
	if _, err = s.Verify(token+"garbage", now); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("tamper accepted: %v", err)
	}
	if _, err = s.Verify(token, now.Add(time.Hour)); !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("expiry accepted: %v", err)
	}
	other, _ := NewSigner([]byte(strings.Repeat("b", 32)))
	if _, err = other.Verify(token, now); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("other key accepted: %v", err)
	}
	if _, err = s.Verify("g1.invalid.signature", now); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("forged token accepted: %v", err)
	}
}
