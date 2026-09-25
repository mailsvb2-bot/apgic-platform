package connector

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidWebhook          = errors.New("invalid connector webhook envelope")
	ErrWebhookKeyUnavailable   = errors.New("connector webhook public key is unavailable")
	ErrWebhookSignatureInvalid = errors.New("connector webhook signature is invalid")
)

type WebhookEnvelope struct {
	ConnectorInstanceID string          `json:"connector_instance_id"`
	ExternalEventID     string          `json:"external_event_id"`
	StreamID            string          `json:"stream_id"`
	Sequence            uint64          `json:"sequence"`
	KeyID               string          `json:"key_id"`
	OccurredAt          time.Time       `json:"occurred_at"`
	Payload             json.RawMessage `json:"payload"`
	Signature           string          `json:"signature"`
}

type webhookSigningPayload struct {
	ConnectorInstanceID string          `json:"connector_instance_id"`
	ExternalEventID     string          `json:"external_event_id"`
	StreamID            string          `json:"stream_id"`
	Sequence            uint64          `json:"sequence"`
	KeyID               string          `json:"key_id"`
	OccurredAt          time.Time       `json:"occurred_at"`
	Payload             json.RawMessage `json:"payload"`
}

type WebhookPublicKeyResolver interface {
	ResolveWebhookPublicKey(connectorInstanceID, keyID string) (ed25519.PublicKey, bool)
}

func (e WebhookEnvelope) Validate() error {
	if strings.TrimSpace(e.ConnectorInstanceID) == "" ||
		strings.TrimSpace(e.ExternalEventID) == "" ||
		strings.TrimSpace(e.StreamID) == "" ||
		e.Sequence == 0 ||
		strings.TrimSpace(e.KeyID) == "" ||
		e.OccurredAt.IsZero() ||
		len(e.Payload) == 0 ||
		!json.Valid(e.Payload) ||
		strings.TrimSpace(e.Signature) == "" {
		return ErrInvalidWebhook
	}
	return nil
}

func webhookSigningBytes(envelope WebhookEnvelope) ([]byte, error) {
	payload := webhookSigningPayload{
		ConnectorInstanceID: envelope.ConnectorInstanceID,
		ExternalEventID:     envelope.ExternalEventID,
		StreamID:            envelope.StreamID,
		Sequence:            envelope.Sequence,
		KeyID:               envelope.KeyID,
		OccurredAt:          envelope.OccurredAt,
		Payload:             envelope.Payload,
	}
	return json.Marshal(payload)
}

func VerifyWebhook(envelope WebhookEnvelope, resolver WebhookPublicKeyResolver) error {
	if err := envelope.Validate(); err != nil {
		return err
	}
	if resolver == nil {
		return ErrWebhookKeyUnavailable
	}
	publicKey, ok := resolver.ResolveWebhookPublicKey(envelope.ConnectorInstanceID, envelope.KeyID)
	if !ok || len(publicKey) != ed25519.PublicKeySize {
		return ErrWebhookKeyUnavailable
	}
	signature, err := base64.StdEncoding.DecodeString(envelope.Signature)
	if err != nil {
		return ErrWebhookSignatureInvalid
	}
	message, err := webhookSigningBytes(envelope)
	if err != nil {
		return ErrInvalidWebhook
	}
	if !ed25519.Verify(publicKey, message, signature) {
		return ErrWebhookSignatureInvalid
	}
	return nil
}

type DeliveryDecision string

const (
	DeliveryApply     DeliveryDecision = "APPLY"
	DeliveryDuplicate DeliveryDecision = "DUPLICATE"
	DeliveryStale     DeliveryDecision = "STALE"
	DeliveryDefer     DeliveryDecision = "DEFER"
)

func DecideDelivery(
	lastAppliedSequence uint64,
	appliedEventIDs map[string]struct{},
	envelope WebhookEnvelope,
) DeliveryDecision {
	if _, ok := appliedEventIDs[envelope.ExternalEventID]; ok {
		return DeliveryDuplicate
	}
	if envelope.Sequence <= lastAppliedSequence {
		return DeliveryStale
	}
	if envelope.Sequence == lastAppliedSequence+1 {
		return DeliveryApply
	}
	return DeliveryDefer
}
