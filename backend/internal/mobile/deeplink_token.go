package mobile

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/authz"
)

const (
	deepLinkTokenVersion = "v1"
	minDeepLinkKeyBytes  = 32
	MaxDeepLinkLifetime  = 24 * time.Hour
)

var (
	ErrDeepLinkSigningKeyInvalid = errors.New("deep link signing key must be at least 32 bytes")
	ErrDeepLinkTokenInvalid      = errors.New("deep link token is invalid")
	ErrDeepLinkTokenExpired      = errors.New("deep link token is expired")
)

type DeepLinkResource struct {
	Kind              LinkKind
	TargetID          string
	AccessClass       LinkAccessClass
	TenantID          string
	SubjectIdentityID string
}

type DeepLinkTokenClaims struct {
	LinkID            string
	Kind              LinkKind
	TargetID          string
	AccessClass       LinkAccessClass
	TenantID          string
	SubjectIdentityID string
	IssuedAt          time.Time
	ExpiresAt         time.Time
}

type deepLinkTokenPayload struct {
	LinkID            string          `json:"link_id"`
	Kind              LinkKind        `json:"kind"`
	TargetID          string          `json:"target_id"`
	AccessClass       LinkAccessClass `json:"access_class"`
	TenantID          string          `json:"tenant_id,omitempty"`
	SubjectIdentityID string          `json:"subject_identity_id,omitempty"`
	IssuedAtUnix      int64           `json:"issued_at"`
	ExpiresAtUnix     int64           `json:"expires_at"`
}

type DeepLinkTokenManager struct {
	key []byte
}

func NewDeepLinkTokenManager(key []byte) (*DeepLinkTokenManager, error) {
	if len(key) < minDeepLinkKeyBytes {
		return nil, ErrDeepLinkSigningKeyInvalid
	}
	return &DeepLinkTokenManager{key: append([]byte(nil), key...)}, nil
}

func (m *DeepLinkTokenManager) Issue(
	resource DeepLinkResource,
	linkID string,
	now time.Time,
	expiresAt time.Time,
) (string, error) {
	now = now.UTC()
	expiresAt = expiresAt.UTC()
	if m == nil ||
		strings.TrimSpace(linkID) == "" ||
		now.IsZero() ||
		!expiresAt.After(now) ||
		expiresAt.Sub(now) > MaxDeepLinkLifetime ||
		!validDeepLinkResource(resource) {
		return "", ErrDeepLinkTokenInvalid
	}
	payload := deepLinkTokenPayload{
		LinkID:            linkID,
		Kind:              resource.Kind,
		TargetID:          resource.TargetID,
		AccessClass:       resource.AccessClass,
		TenantID:          resource.TenantID,
		SubjectIdentityID: resource.SubjectIdentityID,
		IssuedAtUnix:      now.Unix(),
		ExpiresAtUnix:     expiresAt.Unix(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", ErrDeepLinkTokenInvalid
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	signed := deepLinkTokenVersion + "." + encoded
	signature := base64.RawURLEncoding.EncodeToString(m.sign(signed))
	return signed + "." + signature, nil
}

func (m *DeepLinkTokenManager) Parse(token string, now time.Time) (DeepLinkTokenClaims, error) {
	if m == nil || now.IsZero() {
		return DeepLinkTokenClaims{}, ErrDeepLinkTokenInvalid
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != deepLinkTokenVersion {
		return DeepLinkTokenClaims{}, ErrDeepLinkTokenInvalid
	}
	provided, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(provided, m.sign(parts[0]+"."+parts[1])) {
		return DeepLinkTokenClaims{}, ErrDeepLinkTokenInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return DeepLinkTokenClaims{}, ErrDeepLinkTokenInvalid
	}
	var payload deepLinkTokenPayload
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return DeepLinkTokenClaims{}, ErrDeepLinkTokenInvalid
	}
	if decoder.Decode(&struct{}{}) == nil {
		return DeepLinkTokenClaims{}, ErrDeepLinkTokenInvalid
	}
	claims := DeepLinkTokenClaims{
		LinkID:            payload.LinkID,
		Kind:              payload.Kind,
		TargetID:          payload.TargetID,
		AccessClass:       payload.AccessClass,
		TenantID:          payload.TenantID,
		SubjectIdentityID: payload.SubjectIdentityID,
		IssuedAt:          time.Unix(payload.IssuedAtUnix, 0).UTC(),
		ExpiresAt:         time.Unix(payload.ExpiresAtUnix, 0).UTC(),
	}
	resource := DeepLinkResource{
		Kind:              claims.Kind,
		TargetID:          claims.TargetID,
		AccessClass:       claims.AccessClass,
		TenantID:          claims.TenantID,
		SubjectIdentityID: claims.SubjectIdentityID,
	}
	if strings.TrimSpace(claims.LinkID) == "" ||
		payload.IssuedAtUnix <= 0 ||
		payload.ExpiresAtUnix <= payload.IssuedAtUnix ||
		claims.ExpiresAt.Sub(claims.IssuedAt) > MaxDeepLinkLifetime ||
		!validDeepLinkResource(resource) {
		return DeepLinkTokenClaims{}, ErrDeepLinkTokenInvalid
	}
	if !claims.ExpiresAt.After(now.UTC()) {
		return DeepLinkTokenClaims{}, ErrDeepLinkTokenExpired
	}
	return claims, nil
}

func ResolveTrustedDeepLink(
	tokenClaims DeepLinkTokenClaims,
	current DeepLinkResource,
	principal authz.Principal,
	now time.Time,
) DeepLinkResolution {
	if !sameDeepLinkResource(tokenClaims, current) {
		return DeepLinkResolution{Allowed: false, ReasonCode: ReasonLinkInvalid}
	}
	path, ok := canonicalPathFor(current.Kind, current.TargetID)
	if !ok {
		return DeepLinkResolution{Allowed: false, ReasonCode: ReasonLinkInvalid}
	}
	return ResolveDeepLink(DeepLinkClaims{
		LinkID:            tokenClaims.LinkID,
		Kind:              current.Kind,
		AccessClass:       current.AccessClass,
		TargetID:          current.TargetID,
		TenantID:          current.TenantID,
		SubjectIdentityID: current.SubjectIdentityID,
		CanonicalPath:     path,
		WebFallback:       "https://apgic.ru" + path,
		ExpiresAt:         tokenClaims.ExpiresAt,
	}, principal, now)
}

func validDeepLinkResource(resource DeepLinkResource) bool {
	if !validLinkKind(resource.Kind) ||
		!validAccessClass(resource.AccessClass) ||
		!validCanonicalTargetID(resource.TargetID) {
		return false
	}
	switch resource.Kind {
	case LinkBooking, LinkNotification:
		return resource.AccessClass == LinkProtectedResource &&
			strings.TrimSpace(resource.TenantID) != "" &&
			strings.TrimSpace(resource.SubjectIdentityID) != ""
	case LinkSpecialist:
		return resource.AccessClass == LinkPublicResource &&
			strings.TrimSpace(resource.TenantID) == "" &&
			strings.TrimSpace(resource.SubjectIdentityID) == ""
	default:
		return false
	}
}

func sameDeepLinkResource(claims DeepLinkTokenClaims, resource DeepLinkResource) bool {
	return claims.Kind == resource.Kind &&
		claims.TargetID == resource.TargetID &&
		claims.AccessClass == resource.AccessClass &&
		claims.TenantID == resource.TenantID &&
		claims.SubjectIdentityID == resource.SubjectIdentityID &&
		validDeepLinkResource(resource)
}

func (m *DeepLinkTokenManager) sign(value string) []byte {
	mac := hmac.New(sha256.New, m.key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}
