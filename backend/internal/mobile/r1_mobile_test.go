package mobile

import (
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/authz"
)

func principal(id, tenant string, permissions ...string) authz.Principal {
	set := make(map[string]struct{}, len(permissions))
	for _, permission := range permissions {
		set[permission] = struct{}{}
	}
	return authz.Principal{ID: id, TenantID: tenant, Permissions: set}
}

func TestProtectedDeepLinkRechecksServerAuthorization(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	claims := DeepLinkClaims{
		LinkID:            "link-1",
		Kind:              LinkBooking,
		AccessClass:       LinkProtectedResource,
		TargetID:          "booking-1",
		TenantID:          "tenant-a",
		SubjectIdentityID: "identity-1",
		CanonicalPath:     "/bookings/booking-1",
		WebFallback:       "https://apgic.ru/bookings/booking-1",
		ExpiresAt:         now.Add(time.Hour),
	}

	allowed := ResolveDeepLink(
		claims,
		principal("identity-1", "tenant-a", "deeplink.open.booking"),
		now,
	)
	if !allowed.Allowed || allowed.CanonicalPath != claims.CanonicalPath || !allowed.ExpiresAt.Equal(claims.ExpiresAt) {
		t.Fatalf("allowed resolution = %#v", allowed)
	}

	crossTenant := ResolveDeepLink(
		claims,
		principal("identity-1", "tenant-b", "deeplink.open.booking"),
		now,
	)
	if crossTenant.Allowed || crossTenant.ReasonCode != ReasonLinkAuthorizationDeny+":AUTH_CROSS_TENANT_DENY" {
		t.Fatalf("cross-tenant resolution = %#v", crossTenant)
	}

	noPermission := ResolveDeepLink(
		claims,
		principal("identity-1", "tenant-a"),
		now,
	)
	if noPermission.Allowed || noPermission.ReasonCode != ReasonLinkAuthorizationDeny+":AUTH_PERMISSION_DENIED" {
		t.Fatalf("permission resolution = %#v", noPermission)
	}
}

func TestDeepLinkExpirySubjectAndCanonicalFallbackFailClosed(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	base := DeepLinkClaims{
		LinkID:        "link-2",
		Kind:          LinkNotification,
		AccessClass:   LinkProtectedResource,
		TargetID:      "notification-1",
		TenantID:      "tenant-a",
		CanonicalPath: "/notifications/notification-1",
		WebFallback:   "https://apgic.ru/notifications/notification-1",
		ExpiresAt:     now.Add(time.Hour),
	}
	p := principal("identity-1", "tenant-a", "deeplink.open.notification")

	expired := base
	expired.ExpiresAt = now
	if got := ResolveDeepLink(expired, p, now); got.Allowed || got.ReasonCode != ReasonLinkExpired {
		t.Fatalf("expired link = %#v", got)
	}

	wrongSubject := base
	wrongSubject.SubjectIdentityID = "identity-2"
	if got := ResolveDeepLink(wrongSubject, p, now); got.Allowed || got.ReasonCode != ReasonLinkSubjectMismatch {
		t.Fatalf("subject-mismatched link = %#v", got)
	}

	pathConfusion := base
	pathConfusion.CanonicalPath = "/bookings/booking-1"
	pathConfusion.WebFallback = "https://apgic.ru/bookings/booking-1"
	if got := ResolveDeepLink(pathConfusion, p, now); got.Allowed || got.ReasonCode != ReasonLinkInvalid {
		t.Fatalf("kind/path confusion = %#v", got)
	}

	openRedirect := base
	openRedirect.WebFallback = "https://evil.example/notifications/notification-1"
	if got := ResolveDeepLink(openRedirect, p, now); got.Allowed || got.ReasonCode != ReasonLinkFallbackInvalid {
		t.Fatalf("foreign fallback = %#v", got)
	}

	wrongFallbackPath := base
	wrongFallbackPath.WebFallback = "https://apgic.ru/specialists/specialist-1"
	if got := ResolveDeepLink(wrongFallbackPath, p, now); got.Allowed || got.ReasonCode != ReasonLinkFallbackInvalid {
		t.Fatalf("wrong fallback resource = %#v", got)
	}
}

func TestPublicSpecialistLinkStillRequiresValidCanonicalClaims(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	got := ResolveDeepLink(DeepLinkClaims{
		LinkID:        "link-public",
		Kind:          LinkSpecialist,
		AccessClass:   LinkPublicResource,
		TargetID:      "specialist-1",
		CanonicalPath: "/specialists/specialist-1",
		WebFallback:   "https://apgic.ru/specialists/specialist-1",
		ExpiresAt:     now.Add(time.Hour),
	}, authz.Principal{}, now)
	if !got.Allowed {
		t.Fatalf("public specialist link = %#v", got)
	}
}

func TestWorkspaceSwitchNeverTrustsCallerWorkspaceIDAlone(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	workspace := Workspace{
		ID:         "workspace-a",
		IdentityID: "identity-1",
		TenantID:   "tenant-a",
		Kind:       WorkspaceSpecialist,
	}

	allowed := ResolveWorkspace(
		principal("identity-1", "tenant-a", "workspace.open"),
		workspace,
		now,
	)
	if !allowed.Allowed {
		t.Fatalf("workspace should be allowed: %#v", allowed)
	}

	wrongIdentity := workspace
	wrongIdentity.IdentityID = "identity-2"
	if got := ResolveWorkspace(principal("identity-1", "tenant-a", "workspace.open"), wrongIdentity, now); got.Allowed || got.ReasonCode != ReasonWorkspaceIdentityMismatch {
		t.Fatalf("wrong identity workspace = %#v", got)
	}

	if got := ResolveWorkspace(principal("identity-1", "tenant-b", "workspace.open"), workspace, now); got.Allowed || got.ReasonCode != ReasonWorkspaceAuthorization+":AUTH_CROSS_TENANT_DENY" {
		t.Fatalf("cross-tenant workspace = %#v", got)
	}
}
