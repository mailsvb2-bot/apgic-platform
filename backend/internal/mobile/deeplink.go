package mobile

import (
	"net/url"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/authz"
)

type LinkKind string

const (
	LinkBooking      LinkKind = "BOOKING"
	LinkSpecialist   LinkKind = "SPECIALIST"
	LinkNotification LinkKind = "NOTIFICATION"
)

type LinkAccessClass string

const (
	LinkPublicResource    LinkAccessClass = "PUBLIC_RESOURCE"
	LinkProtectedResource LinkAccessClass = "PROTECTED_RESOURCE"
)

const (
	ReasonLinkAllowed           = "DEEPLINK_ALLOWED"
	ReasonLinkInvalid           = "DEEPLINK_INVALID"
	ReasonLinkExpired           = "DEEPLINK_EXPIRED"
	ReasonLinkSubjectMismatch   = "DEEPLINK_SUBJECT_MISMATCH"
	ReasonLinkAuthorizationDeny = "DEEPLINK_AUTHORIZATION_DENY"
	ReasonLinkFallbackInvalid   = "DEEPLINK_FALLBACK_INVALID"
)

type DeepLinkClaims struct {
	LinkID            string
	Kind              LinkKind
	AccessClass       LinkAccessClass
	TargetID          string
	TenantID          string
	SubjectIdentityID string
	CanonicalPath     string
	WebFallback       string
	ExpiresAt         time.Time
}

type DeepLinkResolution struct {
	Allowed       bool
	ReasonCode    string
	CanonicalPath string
	WebFallback   string
	ExpiresAt     time.Time
}

func ResolveDeepLink(claims DeepLinkClaims, principal authz.Principal, now time.Time) DeepLinkResolution {
	deny := func(reason string) DeepLinkResolution {
		return DeepLinkResolution{Allowed: false, ReasonCode: reason}
	}

	if strings.TrimSpace(claims.LinkID) == "" ||
		strings.TrimSpace(claims.TargetID) == "" ||
		strings.TrimSpace(claims.CanonicalPath) == "" ||
		now.IsZero() ||
		!validLinkKind(claims.Kind) ||
		!validAccessClass(claims.AccessClass) {
		return deny(ReasonLinkInvalid)
	}
	expectedPath, ok := canonicalPathFor(claims.Kind, claims.TargetID)
	if !ok || claims.CanonicalPath != expectedPath {
		return deny(ReasonLinkInvalid)
	}
	if !claims.ExpiresAt.After(now) {
		return deny(ReasonLinkExpired)
	}
	if !validCanonicalFallback(claims.WebFallback, expectedPath) {
		return deny(ReasonLinkFallbackInvalid)
	}
	if claims.SubjectIdentityID != "" && claims.SubjectIdentityID != principal.ID {
		return deny(ReasonLinkSubjectMismatch)
	}

	if claims.AccessClass == LinkProtectedResource {
		if strings.TrimSpace(claims.TenantID) == "" {
			return deny(ReasonLinkInvalid)
		}
		action := "deeplink.open." + strings.ToLower(string(claims.Kind))
		result := authz.Authorize(authz.Input{
			Principal: principal,
			Resource: authz.ResourceRef{
				ID:       claims.TargetID,
				TenantID: claims.TenantID,
			},
			Action: action,
			Risk:   authz.RiskNormal,
			Now:    now,
		})
		if result.Decision != authz.Allow {
			return deny(ReasonLinkAuthorizationDeny + ":" + result.ReasonCode)
		}
	}

	return DeepLinkResolution{
		Allowed:       true,
		ReasonCode:    ReasonLinkAllowed,
		CanonicalPath: claims.CanonicalPath,
		WebFallback:   claims.WebFallback,
		ExpiresAt:     claims.ExpiresAt,
	}
}

func validCanonicalFallback(raw, expectedPath string) bool {
	parsed, err := url.Parse(raw)
	if err != nil ||
		parsed.Scheme != "https" ||
		parsed.Opaque != "" ||
		parsed.User != nil ||
		parsed.Path != expectedPath ||
		parsed.RawQuery != "" ||
		parsed.ForceQuery ||
		parsed.Fragment != "" {
		return false
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "apgic.ru" || strings.HasSuffix(host, ".apgic.ru")
}

func validCanonicalTargetID(targetID string) bool {
	if targetID == "" || strings.TrimSpace(targetID) != targetID || targetID == "." || targetID == ".." {
		return false
	}
	// A canonical resource ID must already be a safe URL path segment. Reject
	// percent-encoded separators and other syntax that could be reinterpreted
	// by a router after decoding.
	return url.PathEscape(targetID) == targetID
}

func ValidDeepLinkTarget(kind LinkKind, targetID string) bool {
	return validLinkKind(kind) && validCanonicalTargetID(targetID)
}

func canonicalPathFor(kind LinkKind, targetID string) (string, bool) {
	if !validCanonicalTargetID(targetID) {
		return "", false
	}
	switch kind {
	case LinkBooking:
		return "/bookings/" + targetID, true
	case LinkSpecialist:
		return "/specialists/" + targetID, true
	case LinkNotification:
		return "/notifications/" + targetID, true
	default:
		return "", false
	}
}

func validLinkKind(kind LinkKind) bool {
	switch kind {
	case LinkBooking, LinkSpecialist, LinkNotification:
		return true
	default:
		return false
	}
}

func validAccessClass(class LinkAccessClass) bool {
	switch class {
	case LinkPublicResource, LinkProtectedResource:
		return true
	default:
		return false
	}
}
