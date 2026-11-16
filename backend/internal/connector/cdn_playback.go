package connector

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const (
	CDNPlaybackContractVersion  = "cdn-playback-v2"
	CDNPlaybackMaxGrantLifetime = 15 * time.Minute
	CDNPlaybackMaxDuration      = 24 * time.Hour
	CDNPlaybackClockSkew        = 30 * time.Second
)

var (
	ErrInvalidCDNPlaybackRequest     = errors.New("invalid CDN playback request")
	ErrInvalidCDNPlaybackGrant       = errors.New("invalid CDN playback grant")
	ErrInvalidCDNPlaybackAttestation = errors.New("invalid CDN playback attestation")
)

var (
	cdnPlaybackUUIDPattern        = regexp.MustCompile("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")
	cdnPlaybackProviderPattern    = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$")
	cdnPlaybackIdempotencyPattern = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9._:-]*(/[A-Za-z0-9][A-Za-z0-9._:-]*)*$")
	cdnPlaybackDigestPattern      = regexp.MustCompile("^[0-9a-f]{64}$")
	cdnPlaybackEventIDPattern     = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$")
)

// CDNPlaybackBinding binds provider evidence to the canonical Academy and
// Entitlement records. It contains identifiers and a content fingerprint only;
// no provider URL, OAuth credential, learner answer or payment data is allowed.
type CDNPlaybackBinding struct {
	SubjectID       string `json:"subject_id"`
	EnrollmentID    string `json:"enrollment_id"`
	EntitlementID   string `json:"entitlement_id"`
	CourseID        string `json:"course_id"`
	CourseVersionID string `json:"course_version_id"`
	LessonID        string `json:"lesson_id"`
	MediaID         string `json:"media_id"`
	ContentSHA256   string `json:"content_sha256"`
	ContentLength   int64  `json:"content_length"`
}

func (b CDNPlaybackBinding) Validate() error {
	for _, id := range []string{
		b.SubjectID, b.EnrollmentID, b.EntitlementID, b.CourseID,
		b.CourseVersionID, b.LessonID, b.MediaID,
	} {
		if !cdnPlaybackUUIDPattern.MatchString(id) {
			return ErrInvalidCDNPlaybackRequest
		}
	}
	if !cdnPlaybackDigestPattern.MatchString(b.ContentSHA256) || b.ContentLength <= 0 {
		return ErrInvalidCDNPlaybackRequest
	}
	return nil
}

// CDNPlaybackRequest is the idempotent request to the existing CDN_PROVIDER
// capability. The provider must bind the grant to this exact immutable lesson
// version, learner, enrollment, entitlement and verified media fingerprint.
type CDNPlaybackRequest struct {
	IdempotencyKey     string             `json:"idempotency_key"`
	ProviderInstanceID string             `json:"provider_instance_id"`
	Binding            CDNPlaybackBinding `json:"binding"`
}

func (r CDNPlaybackRequest) Validate() error {
	if !cdnPlaybackIdempotencyKeyValid(r.IdempotencyKey) ||
		!cdnPlaybackUUIDPattern.MatchString(r.ProviderInstanceID) || r.Binding.Validate() != nil {
		return ErrInvalidCDNPlaybackRequest
	}
	return nil
}

func cdnPlaybackIdempotencyKeyValid(value string) bool {
	return len(value) <= 128 && cdnPlaybackIdempotencyPattern.MatchString(value)
}

// CDNPlaybackGrant is an opaque provider session reference, not a public asset
// URL. Browser-specific delivery material is owned and returned by the
// configured provider adapter, never by Academy's persistence/API projection.
type CDNPlaybackGrant struct {
	ContractVersion    string             `json:"contract_version"`
	ProviderKind       string             `json:"provider_kind"`
	ProviderInstanceID string             `json:"provider_instance_id"`
	SessionID          string             `json:"session_id"`
	Binding            CDNPlaybackBinding `json:"binding"`
	IssuedAt           time.Time          `json:"issued_at"`
	ExpiresAt          time.Time          `json:"expires_at"`
}

func (g CDNPlaybackGrant) Validate() error {
	if g.ContractVersion != CDNPlaybackContractVersion ||
		!cdnPlaybackProviderPattern.MatchString(g.ProviderKind) ||
		!cdnPlaybackUUIDPattern.MatchString(g.ProviderInstanceID) ||
		!cdnPlaybackUUIDPattern.MatchString(g.SessionID) ||
		g.Binding.Validate() != nil ||
		!cdnPlaybackUTC(g.IssuedAt) || !cdnPlaybackUTC(g.ExpiresAt) {
		return ErrInvalidCDNPlaybackGrant
	}
	lifetime := g.ExpiresAt.Sub(g.IssuedAt)
	if lifetime <= 0 || lifetime > CDNPlaybackMaxGrantLifetime {
		return ErrInvalidCDNPlaybackGrant
	}
	return nil
}

type CDNPlaybackRange struct {
	StartMillis int64 `json:"start_millis"`
	EndMillis   int64 `json:"end_millis"`
}

// CDNPlaybackAttestation is provider-signed evidence. It reports media ranges
// delivered by a provider session; Academy still applies its own completion
// policy and persists replay protection in the canonical PostgreSQL owner.
type CDNPlaybackAttestation struct {
	ContractVersion    string             `json:"contract_version"`
	ProviderKind       string             `json:"provider_kind"`
	ProviderInstanceID string             `json:"provider_instance_id"`
	ProviderEventID    string             `json:"provider_event_id"`
	SessionID          string             `json:"session_id"`
	Binding            CDNPlaybackBinding `json:"binding"`
	DurationMillis     int64              `json:"duration_millis"`
	WatchedRanges      []CDNPlaybackRange `json:"watched_ranges"`
	StartedAt          time.Time          `json:"started_at"`
	CompletedAt        time.Time          `json:"completed_at"`
	KeyID              string             `json:"key_id"`
	Signature          string             `json:"signature"`
}

type cdnPlaybackAttestationPayload struct {
	ContractVersion    string             `json:"contract_version"`
	ProviderKind       string             `json:"provider_kind"`
	ProviderInstanceID string             `json:"provider_instance_id"`
	ProviderEventID    string             `json:"provider_event_id"`
	SessionID          string             `json:"session_id"`
	Binding            CDNPlaybackBinding `json:"binding"`
	DurationMillis     int64              `json:"duration_millis"`
	WatchedRanges      []CDNPlaybackRange `json:"watched_ranges"`
	StartedAt          time.Time          `json:"started_at"`
	CompletedAt        time.Time          `json:"completed_at"`
	KeyID              string             `json:"key_id"`
}

// SigningBytes is the canonical provider signature payload. Provider adapters
// and independent conformance tests use this exact serialization; the signature
// itself is deliberately excluded from the signed bytes.
func (a CDNPlaybackAttestation) SigningBytes() ([]byte, error) {
	payload := cdnPlaybackAttestationPayload{
		ContractVersion:    a.ContractVersion,
		ProviderKind:       a.ProviderKind,
		ProviderInstanceID: a.ProviderInstanceID,
		ProviderEventID:    a.ProviderEventID,
		SessionID:          a.SessionID,
		Binding:            a.Binding,
		DurationMillis:     a.DurationMillis,
		WatchedRanges:      append([]CDNPlaybackRange(nil), a.WatchedRanges...),
		StartedAt:          a.StartedAt.UTC(),
		CompletedAt:        a.CompletedAt.UTC(),
		KeyID:              a.KeyID,
	}
	return json.Marshal(payload)
}

// VerifiedCDNPlaybackAttestation can only be populated by the verifier in this
// package. Downstream Academy code should persist its binding and evidence
// reference transactionally; a caller-created raw attestation is not trusted.
type VerifiedCDNPlaybackAttestation struct {
	attestation CDNPlaybackAttestation
}

func (v VerifiedCDNPlaybackAttestation) Valid() bool {
	return v.attestation.ContractVersion == CDNPlaybackContractVersion &&
		cdnPlaybackUUIDPattern.MatchString(v.attestation.ProviderInstanceID) &&
		cdnPlaybackUUIDPattern.MatchString(v.attestation.SessionID) &&
		cdnPlaybackEventIDPattern.MatchString(v.attestation.ProviderEventID) &&
		v.attestation.Binding.Validate() == nil && v.attestation.Signature != ""
}

func (v VerifiedCDNPlaybackAttestation) Binding() CDNPlaybackBinding {
	return v.attestation.Binding
}

func (v VerifiedCDNPlaybackAttestation) SessionID() string {
	return v.attestation.SessionID
}

func (v VerifiedCDNPlaybackAttestation) ProviderKind() string {
	return v.attestation.ProviderKind
}

func (v VerifiedCDNPlaybackAttestation) ProviderInstanceID() string {
	return v.attestation.ProviderInstanceID
}

func (v VerifiedCDNPlaybackAttestation) ProviderEventID() string {
	return v.attestation.ProviderEventID
}

func (v VerifiedCDNPlaybackAttestation) KeyID() string {
	return v.attestation.KeyID
}

func (v VerifiedCDNPlaybackAttestation) DurationMilliseconds() int64 {
	return v.attestation.DurationMillis
}

func (v VerifiedCDNPlaybackAttestation) WatchedRanges() []CDNPlaybackRange {
	if !v.Valid() {
		return nil
	}
	return append([]CDNPlaybackRange(nil), v.attestation.WatchedRanges...)
}

// CoversEntireDuration is intentionally stricter than summing watched bytes:
// ranges must cover the complete timeline from zero to duration without gaps.
func (v VerifiedCDNPlaybackAttestation) CoversEntireDuration() bool {
	if !v.Valid() || v.attestation.DurationMillis <= 0 || len(v.attestation.WatchedRanges) == 0 {
		return false
	}
	cursor := int64(0)
	for _, watched := range v.attestation.WatchedRanges {
		if watched.StartMillis != cursor || watched.EndMillis <= watched.StartMillis {
			return false
		}
		cursor = watched.EndMillis
	}
	return cursor == v.attestation.DurationMillis
}

// CompletedSeconds is a completion-only value. Partial playback evidence may
// be audited by a future progress contract but cannot mark a VIDEO lesson done.
func (v VerifiedCDNPlaybackAttestation) CompletedSeconds() int64 {
	if !v.CoversEntireDuration() {
		return 0
	}
	return (v.attestation.DurationMillis + 999) / 1000
}

func (v VerifiedCDNPlaybackAttestation) AttestationJSON() ([]byte, error) {
	if !v.Valid() {
		return nil, ErrInvalidCDNPlaybackAttestation
	}
	return json.Marshal(v.attestation)
}

func (v VerifiedCDNPlaybackAttestation) EvidenceSHA256() (string, error) {
	raw, err := v.AttestationJSON()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func (v VerifiedCDNPlaybackAttestation) EvidenceReference() (string, error) {
	digest, err := v.EvidenceSHA256()
	if err != nil {
		return "", err
	}
	return "sha256:" + digest, nil
}

func (v VerifiedCDNPlaybackAttestation) CompletedAt() time.Time {
	return v.attestation.CompletedAt
}

func (v VerifiedCDNPlaybackAttestation) WatchedMilliseconds() int64 {
	var total int64
	if !v.Valid() {
		return 0
	}
	for _, watched := range v.attestation.WatchedRanges {
		total += watched.EndMillis - watched.StartMillis
	}
	return total
}

// VerifyCDNPlaybackAttestation verifies provider signature and exact grant
// binding. The public key must come from the active trusted CDN connector
// configuration; never accept a key carried by the attestation itself.
func VerifyCDNPlaybackAttestation(
	grant CDNPlaybackGrant,
	attestation CDNPlaybackAttestation,
	trustedKeyID string,
	publicKey ed25519.PublicKey,
	now time.Time,
) (VerifiedCDNPlaybackAttestation, error) {
	if grant.Validate() != nil || trustedKeyID == "" ||
		attestation.ContractVersion != CDNPlaybackContractVersion ||
		!cdnPlaybackProviderPattern.MatchString(attestation.ProviderKind) ||
		!cdnPlaybackEventIDPattern.MatchString(attestation.ProviderEventID) ||
		attestation.ProviderKind != grant.ProviderKind ||
		!cdnPlaybackUUIDPattern.MatchString(attestation.ProviderInstanceID) ||
		attestation.ProviderInstanceID != grant.ProviderInstanceID ||
		attestation.SessionID != grant.SessionID ||
		attestation.Binding != grant.Binding ||
		attestation.DurationMillis <= 0 ||
		attestation.DurationMillis > int64(CDNPlaybackMaxDuration/time.Millisecond) ||
		len(attestation.WatchedRanges) == 0 || len(attestation.WatchedRanges) > 10000 ||
		!cdnPlaybackUTC(attestation.StartedAt) ||
		!cdnPlaybackUTC(attestation.CompletedAt) ||
		!cdnPlaybackUTC(now) ||
		now.UTC().After(grant.ExpiresAt.Add(CDNPlaybackClockSkew)) ||
		attestation.StartedAt.Before(grant.IssuedAt) ||
		attestation.CompletedAt.Before(attestation.StartedAt) ||
		attestation.CompletedAt.After(grant.ExpiresAt) ||
		attestation.CompletedAt.After(now.UTC().Add(CDNPlaybackClockSkew)) ||
		!cdnPlaybackProviderPattern.MatchString(attestation.KeyID) ||
		attestation.KeyID != trustedKeyID ||
		len(publicKey) != ed25519.PublicKeySize ||
		strings.IndexFunc(attestation.Signature, unicode.IsSpace) >= 0 {
		return VerifiedCDNPlaybackAttestation{}, ErrInvalidCDNPlaybackAttestation
	}
	if attestation.Binding.Validate() != nil {
		return VerifiedCDNPlaybackAttestation{}, ErrInvalidCDNPlaybackAttestation
	}
	var previousEnd int64
	for i, watched := range attestation.WatchedRanges {
		if watched.StartMillis < 0 || watched.EndMillis <= watched.StartMillis ||
			watched.EndMillis > attestation.DurationMillis ||
			(i > 0 && watched.StartMillis < previousEnd) {
			return VerifiedCDNPlaybackAttestation{}, ErrInvalidCDNPlaybackAttestation
		}
		previousEnd = watched.EndMillis
	}
	signature, err := base64.StdEncoding.DecodeString(attestation.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return VerifiedCDNPlaybackAttestation{}, ErrInvalidCDNPlaybackAttestation
	}
	message, err := attestation.SigningBytes()
	if err != nil || !ed25519.Verify(publicKey, message, signature) {
		return VerifiedCDNPlaybackAttestation{}, ErrInvalidCDNPlaybackAttestation
	}
	attestation.WatchedRanges = append([]CDNPlaybackRange(nil), attestation.WatchedRanges...)
	attestation.StartedAt = attestation.StartedAt.UTC()
	attestation.CompletedAt = attestation.CompletedAt.UTC()
	return VerifiedCDNPlaybackAttestation{attestation: attestation}, nil
}

func cdnPlaybackUTC(value time.Time) bool {
	if value.IsZero() {
		return false
	}
	_, offset := value.Zone()
	return offset == 0
}

// CDNPlaybackCapability extends the existing provider-neutral connector
// contract. It is not a new media owner and must be registered as CDN_PROVIDER.
type CDNPlaybackCapability interface {
	Provider
	IssuePlaybackGrant(context.Context, CDNPlaybackRequest) (CDNPlaybackGrant, error)
	// OpenPlaybackStream returns only a bounded byte stream and validated
	// representation metadata. Provider URLs, credentials, redirects and
	// response headers are never returned to the learner-facing HTTP layer.
	// A nil range requests the complete object (200); a non-nil range requests
	// one inclusive byte interval (206). Unsatisfiable ranges are rejected by
	// the APGIC gateway as 416 before this method is called.
	OpenPlaybackStream(context.Context, CDNPlaybackGrant, *CDNPlaybackByteRange) (CDNPlaybackStream, error)
	// RevokePlaybackGrant must be idempotent and synchronously close every
	// already-open stream belonging to the exact grant, not only deny new opens.
	RevokePlaybackGrant(context.Context, CDNPlaybackGrant) error
	VerifyPlaybackAttestation(context.Context, CDNPlaybackGrant, CDNPlaybackAttestation) (VerifiedCDNPlaybackAttestation, error)
}

// Ensure the existing canonical capability is the only accepted class.
func cdnPlaybackAttestationMatches(verified, received CDNPlaybackAttestation) bool {
	verifiedPayload, verifiedErr := verified.SigningBytes()
	receivedPayload, receivedErr := received.SigningBytes()
	return verifiedErr == nil && receivedErr == nil &&
		bytes.Equal(verifiedPayload, receivedPayload) && verified.Signature == received.Signature
}

func ValidateCDNPlaybackProvider(provider Provider) error {
	if provider == nil || !cdnPlaybackProviderPattern.MatchString(provider.Kind()) ||
		!providerHasCapability(provider, CapabilityCDN) {
		return errors.New("provider does not expose CDN_PROVIDER capability")
	}
	if _, ok := provider.(CDNPlaybackCapability); !ok {
		return errors.New("CDN_PROVIDER lacks versioned byte delivery, evidence and active-session revocation capability")
	}
	return nil
}
