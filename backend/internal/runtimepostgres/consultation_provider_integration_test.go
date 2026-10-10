package runtimepostgres

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/connector"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

// Run only against an isolated, migrated PostgreSQL integration database.
// An external provider is simulated cryptographically; no funds move.
func TestSignedConsultationPersistsAcrossAPIStoreRestart(t *testing.T) {
	databaseURL := os.Getenv("APGIC_CONNECTOR_DELIVERY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("isolated connector integration database is not configured")
	}
	ctx := context.Background()
	store, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service, err := demand.NewConformanceServiceWithStores(nil, store, store)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := service.CreateIntent("бессонница")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ConfirmIntent(intent.ID, []string{"sleep"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	slots, err := service.Slots("spec-lebedeva")
	if err != nil || len(slots) == 0 {
		t.Fatalf("slots=%d error=%v", len(slots), err)
	}
	hold, err := service.AcquireHold(intent.ID, slots[0].ID, intent.ClientIdentityID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.CreateCheckout(hold.ID, intent.ClientIdentityID, "SBP"); err != nil {
		t.Fatal(err)
	}
	// Represents a prior trusted payment-provider capture in the isolated fixture.
	if _, err = store.db.ExecContext(ctx,
		"UPDATE bookings SET state=$1,updated_at=now() WHERE id=$2::uuid", booking.StateConfirmed, hold.BookingID); err != nil {
		t.Fatal(err)
	}
	connectorID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.db.ExecContext(ctx,
		`INSERT INTO connector_instances(id,capability_class,provider_kind,status,config_ref)
          VALUES($1::uuid,'COMMUNICATION_PROVIDER','consultation-integration','ACTIVE',$2)`,
		connectorID, "secretref://consultation/"+connectorID); err != nil {
		t.Fatal(err)
	}
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	keys := connectorWebhookKeyMap{connectorID + "/test-key": public}
	now := time.Now().UTC().Truncate(time.Microsecond).Add(time.Minute)
	signed := func(seq uint64, factType, role string) connector.WebhookEnvelope {
		payload, e := json.Marshal(ConsultationProviderEvent{
			BookingID: hold.BookingID, FactType: factType, Role: role,
			EvidenceRef:       "provider-evidence/" + factType + "/" + role,
			ProviderReference: "provider-room/" + hold.BookingID,
		})
		if e != nil {
			t.Fatal(e)
		}
		raw := connectorWebhookSigningPayload{
			ConnectorInstanceID: connectorID,
			ExternalEventID:     "consultation-event-" + hold.BookingID + "-" + factType + "-" + role,
			StreamID:            "consultation/" + hold.BookingID,
			Sequence:            seq, KeyID: "test-key", OccurredAt: now.Add(time.Duration(seq) * time.Second), Payload: payload,
		}
		message, e := json.Marshal(raw)
		if e != nil {
			t.Fatal(e)
		}
		return connector.WebhookEnvelope{
			ConnectorInstanceID: raw.ConnectorInstanceID, ExternalEventID: raw.ExternalEventID,
			StreamID: raw.StreamID, Sequence: raw.Sequence, KeyID: raw.KeyID, OccurredAt: raw.OccurredAt,
			Payload: raw.Payload, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(private, message)),
		}
	}
	for _, event := range []connector.WebhookEnvelope{
		signed(1, "JOINED", "CLIENT"), signed(2, "JOINED", "SPECIALIST"),
		signed(3, "STARTED", "SYSTEM"), signed(4, "ENDED", "SYSTEM"),
	} {
		if event.Sequence == 1 {
			forged := event
			forged.Signature = base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
			if _, err = store.ApplyConsultationProviderWebhook(ctx, forged, keys); err == nil {
				t.Fatal("unsigned/forged consultation event accepted")
			}
		}
		decision, e := store.ApplyConsultationProviderWebhook(ctx, event, keys)
		if e != nil || decision != connector.DeliveryApply {
			t.Fatalf("event %d decision=%s err=%v", event.Sequence, decision, e)
		}
		if event.Sequence == 4 {
			decision, e = store.ApplyConsultationProviderWebhook(ctx, event, keys)
			if e != nil || decision != connector.DeliveryDuplicate {
				t.Fatalf("replay decision=%s err=%v", decision, e)
			}
		}
	}
	// The next Checker is a separate connection, equivalent to restarting API
	// without any in-process consultation sessions.
	restarted, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	result, err := restarted.ReadConsultationResult(ctx, hold.BookingID, intent.ClientIdentityID)
	if err != nil || result == nil || result.State != "COMPLETED" || result.CompletionEvidenceRef != "provider-evidence/ENDED/SYSTEM" {
		t.Fatalf("restart result=%#v err=%v", result, err)
	}
	stranger, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	unauthorized, err := restarted.ReadConsultationResult(ctx, hold.BookingID, stranger)
	if err != nil || unauthorized != nil {
		t.Fatalf("other client result=%#v err=%v", unauthorized, err)
	}
	var count int
	if err = restarted.db.QueryRowContext(ctx,
		"SELECT count(*) FROM consultation_lifecycle_facts f JOIN consultation_sessions s ON s.id=f.session_id WHERE s.booking_id=$1::uuid AND f.fact_type='ENDED'", hold.BookingID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("duplicated completion facts: %d", count)
	}
}