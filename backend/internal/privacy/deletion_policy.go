package privacy

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

var ErrDeletionPolicyUnapproved = errors.New("account erasure policy has no valid independent approval")

// DeletionPolicy is a deployment-supplied, independently signed approval.
// The API must never sign or manufacture its own deletion authorization.
type DeletionPolicy struct {
	Version            string            `json:"version"`
	Scope              string            `json:"scope"`
	ProductionApproved bool              `json:"production_approved"`
	Jurisdiction       string            `json:"jurisdiction"`
	ApprovedAt         time.Time         `json:"approved_at"`
	ExpiresAt          time.Time         `json:"expires_at"`
	DataClasses        map[string]string `json:"data_classes"`
	RetentionReason    string            `json:"retention_reason"`
	ProviderRefs       []string          `json:"provider_refs"`
	NoExternalData     bool              `json:"no_external_data"`
}

// SignedDeletionPolicy intentionally has no private key or signing API.
type SignedDeletionPolicy struct {
	Policy    DeletionPolicy `json:"policy"`
	Signature string         `json:"signature"`
}

var deletionClasses = map[string]string{
	"PROFILE":               "ERASE",
	"HELP_INTENT":           "ANONYMIZE",
	"CONSULTATION_EVIDENCE": "RETAIN_WITH_REASON",
	"FINANCIAL_EVIDENCE":    "RETAIN_WITH_REASON",
	"CLIENT_OUTCOME":        "RETAIN_WITH_REASON",
	"CREDENTIAL":            "ERASE",
}

// VerifyDeletionPolicy enforces an external signer, class coverage and expiry.
// Explicitly cannot use the CI-only retention matrix as production approval.
func VerifyDeletionPolicy(raw []byte, publicKey ed25519.PublicKey, expectedVersion string, now time.Time) (DeletionPolicy, error) {
	var envelope SignedDeletionPolicy
	if len(publicKey) != ed25519.PublicKeySize {
		return DeletionPolicy{}, ErrDeletionPolicyUnapproved
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return DeletionPolicy{}, fmt.Errorf("%w: %v", ErrDeletionPolicyUnapproved, err)
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return DeletionPolicy{}, ErrDeletionPolicyUnapproved
	}
	material, err := json.Marshal(envelope.Policy)
	if err != nil {
		return DeletionPolicy{}, err
	}
	signature, err := base64.StdEncoding.Strict().DecodeString(envelope.Signature)
	if err != nil || !ed25519.Verify(publicKey, material, signature) {
		return DeletionPolicy{}, ErrDeletionPolicyUnapproved
	}
	p := envelope.Policy
	if p.Scope != "PRODUCTION" || !p.ProductionApproved ||
		p.Version == "" || p.Version != expectedVersion ||
		strings.TrimSpace(p.Jurisdiction) == "" ||
		p.ApprovedAt.IsZero() || p.ExpiresAt.IsZero() ||
		p.ApprovedAt.After(now) || !p.ExpiresAt.After(now) ||
		strings.TrimSpace(p.RetentionReason) == "" ||
		len(p.DataClasses) != len(deletionClasses) {
		return DeletionPolicy{}, ErrDeletionPolicyUnapproved
	}
	for name, disposition := range deletionClasses {
		if p.DataClasses[name] != disposition {
			return DeletionPolicy{}, ErrDeletionPolicyUnapproved
		}
	}
	if len(p.ProviderRefs) == 0 && !p.NoExternalData {
		return DeletionPolicy{}, ErrDeletionPolicyUnapproved
	}
	if len(p.ProviderRefs) != 0 && p.NoExternalData {
		return DeletionPolicy{}, ErrDeletionPolicyUnapproved
	}
	seen := make(map[string]bool, len(p.ProviderRefs))
	for _, ref := range p.ProviderRefs {
		if strings.TrimSpace(ref) != ref || ref == "" || seen[ref] || len(ref) > 128 {
			return DeletionPolicy{}, ErrDeletionPolicyUnapproved
		}
		seen[ref] = true
	}
	return p, nil
}
