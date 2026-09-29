package runtimepostgres

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/connector"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

type connectorWebhookKeyMap map[string]ed25519.PublicKey

func (m connectorWebhookKeyMap) ResolveWebhookPublicKey(connectorInstanceID, keyID string) (ed25519.PublicKey, bool) {
	key, ok := m[connectorInstanceID+"/"+keyID]
	return key, ok
}

type connectorWebhookSigningPayload struct {
	ConnectorInstanceID string          `json:"connector_instance_id"`
	ExternalEventID     string          `json:"external_event_id"`
	StreamID            string          `json:"stream_id"`
	Sequence            uint64          `json:"sequence"`
	KeyID               string          `json:"key_id"`
	OccurredAt          time.Time       `json:"occurred_at"`
	Payload             json.RawMessage `json:"payload"`
}

func TestConnectorWebhookAppliesBusinessEffectExactlyOnceAcrossOutOfOrderAndRetry(t *testing.T) {
	databaseURL := os.Getenv("APGIC_CONNECTOR_DELIVERY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("connector delivery integration database not configured")
	}
	store, err := Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	connectorID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`
		INSERT INTO connector_instances (
			id, capability_class, provider_kind, status, config_ref
		) VALUES ($1::uuid, 'COMMUNICATION_PROVIDER', 'integration-webhook', 'ACTIVE', $2)
	`, connectorID, "secretref://connector/"+connectorID); err != nil {
		t.Fatal(err)
	}

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	const keyID = "provider-key-v1"
	resolver := connectorWebhookKeyMap{connectorID + "/" + keyID: publicKey}
	streamID := "booking/" + connectorID
	now := time.Now().UTC().Truncate(time.Microsecond)

	auditIDs := make([]string, 4)
	for i := range auditIDs {
		auditIDs[i], err = persistentid.New()
		if err != nil {
			t.Fatal(err)
		}
	}
	eventIDs := []string{"event-1-" + connectorID, "event-2-" + connectorID, "event-3-" + connectorID, "event-4-" + connectorID}

	envelope := func(sequence uint64) connector.WebhookEnvelope {
		payload, marshalErr := json.Marshal(map[string]any{
			"audit_id": auditIDs[sequence-1],
			"sequence": sequence,
		})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		unsigned := connectorWebhookSigningPayload{
			ConnectorInstanceID: connectorID,
			ExternalEventID:     eventIDs[sequence-1],
			StreamID:            streamID,
			Sequence:            sequence,
			KeyID:               keyID,
			OccurredAt:          now.Add(time.Duration(sequence) * time.Second),
			Payload:             payload,
		}
		message, marshalErr := json.Marshal(unsigned)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		return connector.WebhookEnvelope{
			ConnectorInstanceID: unsigned.ConnectorInstanceID,
			ExternalEventID:     unsigned.ExternalEventID,
			StreamID:            unsigned.StreamID,
			Sequence:            unsigned.Sequence,
			KeyID:               unsigned.KeyID,
			OccurredAt:          unsigned.OccurredAt,
			Payload:             unsigned.Payload,
			Signature:           base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, message)),
		}
	}

	applyAuditEffect := func(ctx context.Context, tx interface {
		ExecContext(context.Context, string, ...any) (sqlResult, error)
	}, payload json.RawMessage) error {
		var effect struct {
			AuditID  string `json:"audit_id"`
			Sequence uint64 `json:"sequence"`
		}
		if err := json.Unmarshal(payload, &effect); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO audit_records (
				id, actor_id, action, scope, reason, policy_version, correlation_id
			) VALUES (
				$1::uuid, 'connector-test', 'connector.business_effect', 'connector-test',
				'WEBHOOK_APPLIED', 'connector-v1', $2
			)
		`, effect.AuditID, eventIDs[effect.Sequence-1])
		return err
	}
	_ = applyAuditEffect

	apply := connectorEffectInsertAudit(t, eventIDs)

	decision, err := store.ProcessConnectorWebhook(context.Background(), envelope(2), resolver, apply)
	if err != nil || decision != connector.DeliveryDefer {
		t.Fatalf("sequence 2 decision=%s err=%v", decision, err)
	}
	assertConnectorAuditCount(t, store, auditIDs[1], 0)

	decision, err = store.ProcessConnectorWebhook(context.Background(), envelope(1), resolver, apply)
	if err != nil || decision != connector.DeliveryApply {
		t.Fatalf("sequence 1 decision=%s err=%v", decision, err)
	}
	assertConnectorAuditCount(t, store, auditIDs[0], 1)
	assertConnectorStreamPosition(t, store, connectorID, streamID, 1)

	decision, err = store.ProcessConnectorWebhook(context.Background(), envelope(1), resolver, apply)
	if err != nil || decision != connector.DeliveryDuplicate {
		t.Fatalf("duplicate sequence 1 decision=%s err=%v", decision, err)
	}
	assertConnectorAuditCount(t, store, auditIDs[0], 1)

	promoted, ok, err := store.ApplyNextDeferredConnectorWebhook(context.Background(), connectorID, streamID, apply)
	if err != nil || !ok || promoted != eventIDs[1] {
		t.Fatalf("promoted=%q ok=%v err=%v", promoted, ok, err)
	}
	assertConnectorAuditCount(t, store, auditIDs[1], 1)
	assertConnectorStreamPosition(t, store, connectorID, streamID, 2)

	decision, err = store.ProcessConnectorWebhook(context.Background(), envelope(2), resolver, apply)
	if err != nil || decision != connector.DeliveryDuplicate {
		t.Fatalf("duplicate sequence 2 decision=%s err=%v", decision, err)
	}
	assertConnectorAuditCount(t, store, auditIDs[1], 1)

	decision, err = store.ProcessConnectorWebhook(context.Background(), envelope(4), resolver, apply)
	if err != nil || decision != connector.DeliveryDefer {
		t.Fatalf("sequence 4 decision=%s err=%v", decision, err)
	}

	expectedApplyErr := errors.New("simulated domain failure")
	decision, err = store.ProcessConnectorWebhook(context.Background(), envelope(3), resolver, func(context.Context, *sql.Tx, json.RawMessage) error {
		return expectedApplyErr
	})
	if err == nil || !errors.Is(err, expectedApplyErr) {
		t.Fatalf("sequence 3 failure err=%v", err)
	}
	assertConnectorReceiptCount(t, store, connectorID, eventIDs[2], 0)
	assertConnectorStreamPosition(t, store, connectorID, streamID, 2)

	decision, err = store.ProcessConnectorWebhook(context.Background(), envelope(3), resolver, apply)
	if err != nil || decision != connector.DeliveryApply {
		t.Fatalf("sequence 3 retry decision=%s err=%v", decision, err)
	}
	assertConnectorAuditCount(t, store, auditIDs[2], 1)
	assertConnectorStreamPosition(t, store, connectorID, streamID, 3)

	_, ok, err = store.ApplyNextDeferredConnectorWebhook(context.Background(), connectorID, streamID, func(context.Context, *sql.Tx, json.RawMessage) error {
		return expectedApplyErr
	})
	if err == nil || !errors.Is(err, expectedApplyErr) || ok {
		t.Fatalf("deferred failure ok=%v err=%v", ok, err)
	}
	assertConnectorAuditCount(t, store, auditIDs[3], 0)
	assertConnectorReceiptState(t, store, connectorID, eventIDs[3], "DEFERRED")
	assertConnectorStreamPosition(t, store, connectorID, streamID, 3)

	promoted, ok, err = store.ApplyNextDeferredConnectorWebhook(context.Background(), connectorID, streamID, apply)
	if err != nil || !ok || promoted != eventIDs[3] {
		t.Fatalf("sequence 4 retry promoted=%q ok=%v err=%v", promoted, ok, err)
	}
	assertConnectorAuditCount(t, store, auditIDs[3], 1)
	assertConnectorReceiptState(t, store, connectorID, eventIDs[3], "APPLIED")
	assertConnectorStreamPosition(t, store, connectorID, streamID, 4)
}

func connectorEffectInsertAudit(t *testing.T, eventIDs []string) ConnectorDomainEffect {
	t.Helper()
	return func(ctx context.Context, tx *sql.Tx, payload json.RawMessage) error {
		var effect struct {
			AuditID  string `json:"audit_id"`
			Sequence uint64 `json:"sequence"`
		}
		if err := json.Unmarshal(payload, &effect); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO audit_records (
				id, actor_id, action, scope, reason, policy_version, correlation_id
			) VALUES (
				$1::uuid, 'connector-test', 'connector.business_effect', 'connector-test',
				'WEBHOOK_APPLIED', 'connector-v1', $2
			)
		`, effect.AuditID, eventIDs[effect.Sequence-1])
		return err
	}
}

func assertConnectorAuditCount(t *testing.T, store *Checker, auditID string, want int) {
	t.Helper()
	var got int
	if err := store.db.QueryRow(`SELECT count(*) FROM audit_records WHERE id = $1::uuid`, auditID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("audit count=%d want %d", got, want)
	}
}

func assertConnectorReceiptCount(t *testing.T, store *Checker, connectorID, eventID string, want int) {
	t.Helper()
	var got int
	if err := store.db.QueryRow(`
		SELECT count(*) FROM connector_delivery_receipts
		WHERE connector_instance_id = $1::uuid AND external_event_id = $2
	`, connectorID, eventID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("receipt count=%d want %d", got, want)
	}
}

func assertConnectorReceiptState(t *testing.T, store *Checker, connectorID, eventID, want string) {
	t.Helper()
	var got string
	if err := store.db.QueryRow(`
		SELECT state FROM connector_delivery_receipts
		WHERE connector_instance_id = $1::uuid AND external_event_id = $2
	`, connectorID, eventID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("receipt state=%s want %s", got, want)
	}
}

func assertConnectorStreamPosition(t *testing.T, store *Checker, connectorID, streamID string, want int64) {
	t.Helper()
	var got int64
	if err := store.db.QueryRow(`
		SELECT last_applied_sequence FROM connector_stream_positions
		WHERE connector_instance_id = $1::uuid AND stream_id = $2
	`, connectorID, streamID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("stream position=%d want %d", got, want)
	}
}
