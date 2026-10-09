package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStepUpAssertionIsBoundToIdentitySessionAndMethod(t *testing.T) {
	now := time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC)
	manager, err := newStepUpManager([]byte(strings.Repeat("s", 32)), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := manager.issue("identity-1", "session:one", "WEBAUTHN", now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPatch, "/v1/products/p/commercial-owner", nil)
	request.AddCookie(cookie)

	evidence := manager.evidenceFromRequest(request, "identity-1", "session:one")
	if evidence == nil || evidence.Method != "WEBAUTHN" || evidence.SessionRef != "session:one" {
		t.Fatalf("expected valid bound evidence, got %#v", evidence)
	}
	if manager.evidenceFromRequest(request, "identity-2", "session:one") != nil {
		t.Fatal("step-up assertion must not cross identities")
	}
	if manager.evidenceFromRequest(request, "identity-1", "session:two") != nil {
		t.Fatal("step-up assertion must not cross sessions")
	}
}

func TestStepUpAssertionRejectsExpiryAndInvalidFields(t *testing.T) {
	now := time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC)
	manager, err := newStepUpManager([]byte(strings.Repeat("s", 32)), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.issue("identity-1", "", "WEBAUTHN", now); err != ErrStepUpAssertionInvalid {
		t.Fatalf("empty session ref err=%v", err)
	}
	if _, err := manager.issue("identity-1", "session:one", "", now); err != ErrStepUpAssertionInvalid {
		t.Fatalf("empty method err=%v", err)
	}

	cookie, err := manager.issue("identity-1", "session:one", "TOTP", now.Add(-stepUpTTL-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPatch, "/", nil)
	request.AddCookie(cookie)
	if manager.evidenceFromRequest(request, "identity-1", "session:one") != nil {
		t.Fatal("expired step-up assertion must be rejected")
	}
}
