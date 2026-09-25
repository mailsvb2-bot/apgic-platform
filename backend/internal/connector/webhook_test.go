package connector

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

type webhookKeyMap map[string]ed25519.PublicKey

func (m webhookKeyMap) ResolveWebhookPublicKey(connectorInstanceID, keyID string) (ed25519.PublicKey, bool) {
	key, ok := m[connectorInstanceID+"/"+keyID]
	return key, ok
}

func signedWebhook(t *testing.T) (WebhookEnvelope, ed25519.PublicKey) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	envelope := WebhookEnvelope{
		ConnectorInstanceID: "connector-1",
		ExternalEventID:     "event-1",
		StreamID:            "booking/1",
		Sequence:            1,
		KeyID:               "provider-key-v1",
		OccurredAt:          time.Now().UTC(),
		Payload:             []byte(`{"booking_id":"1","state":"confirmed"}`),
	}
	message, err := webhookSigningBytes(envelope)
	if err != nil {
		t.Fatal(err)
	}
	envelope.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, message))
	return envelope, publicKey
}

func TestWebhookSignatureBindsDeliveryIdentitySequenceAndPayload(t *testing.T) {
	envelope, publicKey := signedWebhook(t)
	resolver := webhookKeyMap{
		"connector-1/provider-key-v1": publicKey,
	}
	if err := VerifyWebhook(envelope, resolver); err != nil {
		t.Fatal(err)
	}

	tampered := envelope
	tampered.Sequence = 2
	if err := VerifyWebhook(tampered, resolver); !errors.Is(err, ErrWebhookSignatureInvalid) {
		t.Fatalf("tampered sequence must fail signature, got %v", err)
	}

	tampered = envelope
	tampered.Payload = []byte(`{"booking_id":"1","state":"cancelled"}`)
	if err := VerifyWebhook(tampered, resolver); !errors.Is(err, ErrWebhookSignatureInvalid) {
		t.Fatalf("tampered payload must fail signature, got %v", err)
	}
}

func TestDeliveryDecisionIsDeterministicForDuplicateStaleAndOutOfOrder(t *testing.T) {
	envelope, _ := signedWebhook(t)
	applied := map[string]struct{}{"already-applied": {}}

	envelope.ExternalEventID = "already-applied"
	if got := DecideDelivery(1, applied, envelope); got != DeliveryDuplicate {
		t.Fatalf("duplicate decision = %s", got)
	}

	envelope.ExternalEventID = "stale-new-id"
	envelope.Sequence = 1
	if got := DecideDelivery(2, applied, envelope); got != DeliveryStale {
		t.Fatalf("stale decision = %s", got)
	}

	envelope.Sequence = 4
	if got := DecideDelivery(2, applied, envelope); got != DeliveryDefer {
		t.Fatalf("out-of-order decision = %s", got)
	}

	envelope.Sequence = 3
	if got := DecideDelivery(2, applied, envelope); got != DeliveryApply {
		t.Fatalf("next sequence decision = %s", got)
	}
}
