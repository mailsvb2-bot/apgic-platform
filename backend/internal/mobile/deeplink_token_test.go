package mobile

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/authz"
)

func TestDeepLinkTokenIsOpaqueAuthenticatedBoundedAndReauthorized(t *testing.T) {
	key := []byte(strings.Repeat("k", 32))
	manager, err := NewDeepLinkTokenManager(key)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	resource := DeepLinkResource{
		Kind:              LinkBooking,
		TargetID:          "00000000-0000-0000-0000-000000000701",
		AccessClass:       LinkProtectedResource,
		TenantID:          "tenant-a",
		SubjectIdentityID: "identity-a",
	}
	token, err := manager.Issue(resource, "link-1", now, now.Add(15*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] != deepLinkTokenVersion {
		t.Fatalf("unexpected opaque token shape: %q", token)
	}
	sealed, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sealed), resource.SubjectIdentityID) ||
		strings.Contains(string(sealed), resource.TenantID) ||
		strings.Contains(string(sealed), resource.TargetID) {
		t.Fatal("encrypted token leaked raw protected claims")
	}

	claims, err := manager.Parse(token, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	resolution := ResolveTrustedDeepLink(claims, resource, authz.Principal{
		ID:       "identity-a",
		TenantID: "tenant-a",
		Permissions: map[string]struct{}{
			"deeplink.open.booking": {},
		},
	}, now.Add(time.Minute))
	if !resolution.Allowed || resolution.CanonicalPath != "/bookings/"+resource.TargetID ||
		resolution.WebFallback != "https://apgic.ru/bookings/"+resource.TargetID {
		t.Fatalf("unexpected resolution: %#v", resolution)
	}

	tamperedBytes := append([]byte(nil), sealed...)
	tamperedBytes[len(tamperedBytes)-1] ^= 0x01
	tampered := deepLinkTokenVersion + "." + base64.RawURLEncoding.EncodeToString(tamperedBytes)
	if _, err := manager.Parse(tampered, now.Add(time.Minute)); !errors.Is(err, ErrDeepLinkTokenInvalid) {
		t.Fatalf("tampered token error=%v", err)
	}
	if _, err := manager.Parse(token, now.Add(16*time.Minute)); !errors.Is(err, ErrDeepLinkTokenExpired) {
		t.Fatalf("expired token error=%v", err)
	}

	changed := resource
	changed.SubjectIdentityID = "identity-b"
	if got := ResolveTrustedDeepLink(claims, changed, authz.Principal{
		ID:       "identity-a",
		TenantID: "tenant-a",
		Permissions: map[string]struct{}{
			"deeplink.open.booking": {},
		},
	}, now.Add(time.Minute)); got.Allowed || got.ReasonCode != ReasonLinkInvalid {
		t.Fatalf("changed canonical ownership must invalidate token: %#v", got)
	}
}

func TestDeepLinkTokenRejectsInvalidResourceAndUnboundedLifetime(t *testing.T) {
	manager, err := NewDeepLinkTokenManager([]byte(strings.Repeat("x", 32)))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	public := DeepLinkResource{
		Kind:        LinkSpecialist,
		TargetID:    "00000000-0000-0000-0000-000000000702",
		AccessClass: LinkPublicResource,
	}
	if _, err := manager.Issue(public, "link-2", now, now.Add(MaxDeepLinkLifetime+time.Second)); !errors.Is(err, ErrDeepLinkTokenInvalid) {
		t.Fatalf("unbounded token lifetime error=%v", err)
	}
	invalid := public
	invalid.SubjectIdentityID = "must-not-be-on-public-link"
	if _, err := manager.Issue(invalid, "link-3", now, now.Add(time.Minute)); !errors.Is(err, ErrDeepLinkTokenInvalid) {
		t.Fatalf("public subject must be rejected: %v", err)
	}
	if _, err := NewDeepLinkTokenManager([]byte("short")); !errors.Is(err, ErrDeepLinkSigningKeyInvalid) {
		t.Fatalf("short key error=%v", err)
	}
}
