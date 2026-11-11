package privacy

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

var ErrProviderErasureUnverified = errors.New("provider erasure has no independently verified signed receipt")

// ProviderErasureAttestation is signed by the EXTERNAL provider, not APGIC.
type ProviderErasureAttestation struct {
	RequestID   string    `json:"request_id"`
	ProviderRef string    `json:"provider_ref"`
	IdentityID  string    `json:"identity_id"`
	EvidenceRef string    `json:"evidence_ref"`
	Erased      bool      `json:"erased"`
	ErasedAt    time.Time `json:"erased_at"`
}

type SignedProviderErasureAttestation struct {
	Attestation ProviderErasureAttestation `json:"attestation"`
	Signature   string                     `json:"signature"`
}

func VerifyProviderErasure(raw []byte, key ed25519.PublicKey, requestID, providerRef, identityID string, now time.Time) (ProviderErasureAttestation, string, error) {
	var out SignedProviderErasureAttestation
	if len(key) != ed25519.PublicKeySize || len(raw) > 8192 {
		return ProviderErasureAttestation{}, "", ErrProviderErasureUnverified
	}
	// Refuse unexpected envelope fields (including nested provider evidence),
	// and concatenated payloads. They must not be silently discarded before
	// a signature over the canonical attestation is checked.
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&out); err != nil {
		return ProviderErasureAttestation{}, "", ErrProviderErasureUnverified
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return ProviderErasureAttestation{}, "", ErrProviderErasureUnverified
	}
	a := out.Attestation
	// Normalize only for validation; preserve the original signed evidence.
	evidenceMarker := strings.ToLower(strings.TrimSpace(a.EvidenceRef))
	if a.RequestID != requestID || a.ProviderRef != providerRef ||
		a.IdentityID != identityID || !a.Erased || strings.TrimSpace(a.EvidenceRef) == "" ||
		len(a.EvidenceRef) > 1024 ||
		strings.HasPrefix(evidenceMarker, "provider-erasure:") ||
		strings.HasPrefix(evidenceMarker, "synthetic:") ||
		a.ErasedAt.IsZero() ||
		a.ErasedAt.After(now.Add(time.Minute)) ||
		now.Sub(a.ErasedAt) > 24*time.Hour {
		return ProviderErasureAttestation{}, "", ErrProviderErasureUnverified
	}
	material, err := json.Marshal(a)
	if err != nil {
		return ProviderErasureAttestation{}, "", err
	}
	signature, err := base64.StdEncoding.Strict().DecodeString(out.Signature)
	if err != nil || !ed25519.Verify(key, material, signature) {
		return ProviderErasureAttestation{}, "", ErrProviderErasureUnverified
	}
	digest := sha256.Sum256(material)
	return a, fmt.Sprintf("%x", digest), nil
}
