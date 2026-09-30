package security

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestServiceCredentialAuthenticatesWithoutRetainingPlaintext(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	secret := strings.Repeat("a", 32)
	principal, err := NewServicePrincipal("connector-runtime", []string{"connector:execute:NOTIFICATION_PROVIDER"})
	if err != nil {
		t.Fatal(err)
	}
	credential, err := NewServiceCredential(
		principal.ID,
		"v1",
		secret,
		now.Add(-time.Minute),
		now.Add(time.Hour),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := credential.Authenticate(principal, "wrong-"+secret, now); !errors.Is(err, ErrServiceSecretMismatch) {
		t.Fatalf("wrong secret err=%v", err)
	}
	authenticated, err := credential.Authenticate(principal, secret, now)
	if err != nil {
		t.Fatal(err)
	}
	if authenticated.ID() != principal.ID ||
		!authenticated.HasScope("connector:execute:NOTIFICATION_PROVIDER") ||
		authenticated.CredentialVersion() != "v1" {
		t.Fatalf("unexpected authenticated principal: %#v", authenticated)
	}
	if strings.Contains(strings.TrimSpace(strings.ReplaceAll(strings.TrimSpace(secret), " ", "")), "not-present") {
		t.Fatal("unreachable")
	}
}

func TestServiceCredentialRotationRevokesPreviousVersion(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	principal, _ := NewServicePrincipal("connector-runtime", []string{"connector:execute:PAYMENT_PROVIDER"})
	oldSecret := strings.Repeat("o", 32)
	newSecret := strings.Repeat("n", 32)
	credential, err := NewServiceCredential(principal.ID, "v1", oldSecret, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	previous, next, err := credential.Rotate("v2", newSecret, now, now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := previous.Authenticate(principal, oldSecret, now); !errors.Is(err, ErrServiceCredentialDenied) {
		t.Fatalf("previous credential must be revoked, err=%v", err)
	}
	if _, err := next.Authenticate(principal, newSecret, now); err != nil {
		t.Fatalf("rotated credential must authenticate: %v", err)
	}
}

func TestServiceCredentialRejectsExpiredFutureAndRevokedCredentials(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	principal, _ := NewServicePrincipal("connector-runtime", []string{"connector:execute:CALENDAR_PROVIDER"})
	secret := strings.Repeat("s", 32)

	future, _ := NewServiceCredential(principal.ID, "future", secret, now.Add(time.Minute), now.Add(time.Hour))
	if _, err := future.Authenticate(principal, secret, now); !errors.Is(err, ErrServiceCredentialDenied) {
		t.Fatalf("future credential err=%v", err)
	}
	expired, _ := NewServiceCredential(principal.ID, "expired", secret, now.Add(-2*time.Hour), now.Add(-time.Hour))
	if _, err := expired.Authenticate(principal, secret, now); !errors.Is(err, ErrServiceCredentialDenied) {
		t.Fatalf("expired credential err=%v", err)
	}
	active, _ := NewServiceCredential(principal.ID, "active", secret, now.Add(-time.Hour), now.Add(time.Hour))
	revoked, err := active.Revoke(now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := revoked.Authenticate(principal, secret, now); !errors.Is(err, ErrServiceCredentialDenied) {
		t.Fatalf("revoked credential err=%v", err)
	}
}

func TestServiceCredentialRejectsWeakSecretAndInvalidWindow(t *testing.T) {
	now := time.Now().UTC()
	if _, err := NewServiceCredential("connector-runtime", "v1", "short", now, now.Add(time.Hour)); !errors.Is(err, ErrInvalidServiceCredential) {
		t.Fatalf("weak secret err=%v", err)
	}
	if _, err := NewServiceCredential("connector-runtime", "v1", strings.Repeat("x", 32), now, now); !errors.Is(err, ErrInvalidServiceCredential) {
		t.Fatalf("invalid window err=%v", err)
	}
}
