package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientSessionRoundTripAndCookieHardening(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	manager, err := newClientSessionManager([]byte(strings.Repeat("k", 32)), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := manager.issue("11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if cookie.Name != clientSessionCookieName || !cookie.HttpOnly || !cookie.Secure || cookie.Path != "/" || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie hardening = %#v", cookie)
	}

	secondCookie, err := manager.issue("11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if secondCookie.Value == cookie.Value {
		t.Fatal("separate client sessions must have unique signed values")
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/help-intents", nil)
	request.AddCookie(cookie)
	identityID, sessionRef, err := manager.identityAndReferenceFromRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if identityID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("identity = %q", identityID)
	}
	if !strings.HasPrefix(sessionRef, "session:") {
		t.Fatalf("session reference = %q", sessionRef)
	}
}

func TestClientSessionRejectsTamperingAndExpiry(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	manager, err := newClientSessionManager([]byte(strings.Repeat("k", 32)), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := manager.issue("11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatal(err)
	}

	tampered := *cookie
	parts := strings.Split(tampered.Value, ".")
	if len(parts) != 3 || len(parts[2]) == 0 {
		t.Fatal("unexpected signed session token shape")
	}
	replacement := byte('A')
	if parts[2][0] == replacement {
		replacement = 'B'
	}
	parts[2] = string(replacement) + parts[2][1:]
	tampered.Value = strings.Join(parts, ".")
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&tampered)
	if _, err := manager.identityFromRequest(request); !errors.Is(err, ErrClientSessionInvalid) {
		t.Fatalf("tampered session err = %v", err)
	}

	manager.now = func() time.Time { return now.Add(clientSessionTTL + time.Second) }
	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(cookie)
	if _, err := manager.identityFromRequest(request); !errors.Is(err, ErrClientSessionExpired) {
		t.Fatalf("expired session err = %v", err)
	}
}

func TestClientSessionMissingCreatesOneIdentityOnly(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	manager, err := newClientSessionManager([]byte(strings.Repeat("k", 32)), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/help-intents", nil)
	identityID, cookie, err := manager.identityForCreate(request)
	if err != nil {
		t.Fatal(err)
	}
	if identityID == "" || cookie == nil {
		t.Fatalf("new session identity=%q cookie=%#v", identityID, cookie)
	}

	replay := httptest.NewRequest(http.MethodPost, "/v1/help-intents", nil)
	replay.AddCookie(cookie)
	reusedIdentityID, newCookie, err := manager.identityForCreate(replay)
	if err != nil {
		t.Fatal(err)
	}
	if reusedIdentityID != identityID || newCookie != nil {
		t.Fatalf("session rotated canonical identity: first=%q replay=%q cookie=%#v", identityID, reusedIdentityID, newCookie)
	}
}

func TestClientSessionKeyIsFailClosed(t *testing.T) {
	if _, err := newClientSessionManager([]byte("short"), time.Now); !errors.Is(err, ErrClientSessionKeyInvalid) {
		t.Fatalf("weak key err = %v", err)
	}
}
