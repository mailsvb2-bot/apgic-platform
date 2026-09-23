package remoteconfig

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidEnvelope     = errors.New("invalid remote config envelope")
	ErrInvalidSignature    = errors.New("invalid remote config signature")
	ErrStaleVersion        = errors.New("remote config version is not monotonic")
	ErrExpiredConfig       = errors.New("remote config expired")
	ErrPrivilegedCapability = errors.New("remote config cannot own privileged business truth")
)

type Capability string

const (
	CapabilityRealtimeConsultation Capability = "REALTIME_CONSULTATION"
	CapabilityCalendarIntegration  Capability = "CALENDAR_INTEGRATION"
	CapabilityPersonaPreview       Capability = "PERSONA_PREVIEW"
)

var privilegedCapabilityNames = map[string]struct{}{
	"PAYMENT_CAPTURE":          {},
	"PAYOUT_ELIGIBILITY":       {},
	"QUALIFICATION_APPROVAL":   {},
	"ENTITLEMENT_GRANT":        {},
	"LEGAL_ACCEPTANCE":         {},
}

type Payload struct {
	Version      uint64              `json:"version"`
	IssuedAt     time.Time           `json:"issued_at"`
	ExpiresAt    time.Time           `json:"expires_at"`
	PolicyID     string              `json:"policy_id"`
	Disabled     []Capability        `json:"disabled_capabilities"`
	ReasonCodes  map[Capability]string `json:"reason_codes,omitempty"`
}

type SignedEnvelope struct {
	KeyID     string  `json:"key_id"`
	Payload   Payload `json:"payload"`
	Signature string  `json:"signature"`
}

func canonicalPayload(payload Payload) ([]byte, error) {
	return json.Marshal(payload)
}

func Sign(payload Payload, keyID string, privateKey ed25519.PrivateKey) (SignedEnvelope, error) {
	if err := validatePayload(payload, time.Time{}); err != nil && !errors.Is(err, ErrExpiredConfig) {
		return SignedEnvelope{}, err
	}
	if strings.TrimSpace(keyID) == "" || len(privateKey) != ed25519.PrivateKeySize {
		return SignedEnvelope{}, ErrInvalidEnvelope
	}
	raw, err := canonicalPayload(payload)
	if err != nil {
		return SignedEnvelope{}, err
	}
	signature := ed25519.Sign(privateKey, raw)
	return SignedEnvelope{
		KeyID:     keyID,
		Payload:   payload,
		Signature: base64.StdEncoding.EncodeToString(signature),
	}, nil
}

func Verify(envelope SignedEnvelope, publicKey ed25519.PublicKey, now time.Time) error {
	if strings.TrimSpace(envelope.KeyID) == "" || len(publicKey) != ed25519.PublicKeySize {
		return ErrInvalidEnvelope
	}
	if err := validatePayload(envelope.Payload, now); err != nil {
		return err
	}
	signature, err := base64.StdEncoding.DecodeString(envelope.Signature)
	if err != nil {
		return ErrInvalidSignature
	}
	raw, err := canonicalPayload(envelope.Payload)
	if err != nil {
		return err
	}
	if !ed25519.Verify(publicKey, raw, signature) {
		return ErrInvalidSignature
	}
	return nil
}

func validatePayload(payload Payload, now time.Time) error {
	if payload.Version == 0 || strings.TrimSpace(payload.PolicyID) == "" ||
		payload.IssuedAt.IsZero() || payload.ExpiresAt.IsZero() ||
		!payload.ExpiresAt.After(payload.IssuedAt) {
		return ErrInvalidEnvelope
	}
	if !now.IsZero() && !now.Before(payload.ExpiresAt) {
		return ErrExpiredConfig
	}
	for _, capability := range payload.Disabled {
		if strings.TrimSpace(string(capability)) == "" {
			return ErrInvalidEnvelope
		}
		if _, forbidden := privilegedCapabilityNames[string(capability)]; forbidden {
			return ErrPrivilegedCapability
		}
	}
	return nil
}

type Manager struct {
	active *SignedEnvelope
}

func (m *Manager) Apply(envelope SignedEnvelope, publicKey ed25519.PublicKey, now time.Time) error {
	if err := Verify(envelope, publicKey, now); err != nil {
		return err
	}
	if m.active != nil && envelope.Payload.Version <= m.active.Payload.Version {
		return ErrStaleVersion
	}
	copy := envelope
	m.active = &copy
	return nil
}

func (m *Manager) Active() (SignedEnvelope, bool) {
	if m.active == nil {
		return SignedEnvelope{}, false
	}
	return *m.active, true
}

func (m *Manager) IsDisabled(capability Capability) bool {
	if m.active == nil {
		return false
	}
	for _, disabled := range m.active.Payload.Disabled {
		if disabled == capability {
			return true
		}
	}
	return false
}
