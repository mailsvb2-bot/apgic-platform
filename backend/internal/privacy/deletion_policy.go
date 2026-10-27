package privacy

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

var ErrDeletionPolicyUnapproved = errors.New("production deletion requires independently signed, approved retention policy")

// ApprovedDeletionPolicy is supplied by an independently approved legal
// authority, never by the client requesting deletion or by the API itself.
type ApprovedDeletionPolicy struct {
	Version                     string            `json:"version"`
	Jurisdiction                string            `json:"jurisdiction"`
	ApprovedBy                  string            `json:"approved_by"`
	ApprovedAt                  time.Time         `json:"approved_at"`
	ExpiresAt                   time.Time         `json:"expires_at"`
	ProductionApproved          bool              `json:"production_approved"`
	DataClasses                 map[string]string `json:"data_classes"`
	FinancialRetentionReason    string            `json:"financial_retention_reason"`
	ConsultationRetentionReason string            `json:"consultation_retention_reason"`
	ConsentRetentionReason      string            `json:"consent_retention_reason"`
	ProviderRefs                []string          `json:"provider_refs"`
	ProviderInventoryEvidence   string            `json:"provider_inventory_evidence"`
}

type VerifiedDeletionPolicy struct {
	policy         ApprovedDeletionPolicy
	raw            []byte
	digest         string
	signature      []byte
	keyFingerprint string
}

func (v VerifiedDeletionPolicy) Policy() ApprovedDeletionPolicy {
	var copy ApprovedDeletionPolicy
	_ = json.Unmarshal(v.raw, &copy)
	return copy
}
func (v VerifiedDeletionPolicy) RawJSON() []byte        { return append([]byte(nil), v.raw...) }
func (v VerifiedDeletionPolicy) Digest() string         { return v.digest }
func (v VerifiedDeletionPolicy) Signature() []byte      { return append([]byte(nil), v.signature...) }
func (v VerifiedDeletionPolicy) KeyFingerprint() string { return v.keyFingerprint }

// VerifyDeletionPolicy enforces a real signature over exact policy bytes.
// A CI-only matrix or an unsigned operator-supplied JSON never enables erasure.
func VerifyDeletionPolicy(raw, signature []byte, key ed25519.PublicKey, now time.Time) (VerifiedDeletionPolicy, error) {
	var v VerifiedDeletionPolicy
	if len(key) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		!ed25519.Verify(key, raw, signature) {
		return v, ErrDeletionPolicyUnapproved
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&v.policy); err != nil {
		return v, ErrDeletionPolicyUnapproved
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return v, ErrDeletionPolicyUnapproved
	}
	p := v.policy
	if !p.ProductionApproved || p.Version == "" || p.Jurisdiction == "" || p.ApprovedBy == "" ||
		p.ApprovedAt.IsZero() || p.ExpiresAt.IsZero() || p.ApprovedAt.After(now) ||
		!p.ExpiresAt.After(now) || p.ExpiresAt.Sub(p.ApprovedAt) > 366*24*time.Hour ||
		strings.TrimSpace(p.FinancialRetentionReason) == "" ||
		strings.TrimSpace(p.ConsultationRetentionReason) == "" ||
		strings.TrimSpace(p.ConsentRetentionReason) == "" ||
		strings.TrimSpace(p.ProviderInventoryEvidence) == "" {
		return VerifiedDeletionPolicy{}, ErrDeletionPolicyUnapproved
	}
	required := map[string]string{
		"PERSONAL_PREFERENCES":      "ERASE",
		"HELP_INTENT_PERSONAL_DATA": "ANONYMIZE",
		"PUSH_ENDPOINTS":            "ERASE",
		"FINANCIAL_EVIDENCE":        "RETAIN_WITH_REASON",
		"CONSULTATION_EVIDENCE":     "RETAIN_WITH_REASON",
		"CONSENT_EVIDENCE":          "RETAIN_WITH_REASON",
	}
	if len(p.DataClasses) != len(required) {
		return VerifiedDeletionPolicy{}, ErrDeletionPolicyUnapproved
	}
	for k, want := range required {
		if p.DataClasses[k] != want {
			return VerifiedDeletionPolicy{}, ErrDeletionPolicyUnapproved
		}
	}
	seen := make(map[string]bool)
	for _, ref := range p.ProviderRefs {
		if ref == "" || strings.TrimSpace(ref) != ref || seen[ref] {
			return VerifiedDeletionPolicy{}, ErrDeletionPolicyUnapproved
		}
		seen[ref] = true
	}
	h := sha256.Sum256(raw)
	v.digest = hex.EncodeToString(h[:])
	v.raw = append([]byte(nil), raw...)
	v.signature = append([]byte(nil), signature...)
	pk := sha256.Sum256(key)
	v.keyFingerprint = hex.EncodeToString(pk[:])
	return v, nil
}
