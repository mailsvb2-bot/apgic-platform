package runtimepostgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/ledger"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

func TestCoreJourneyContinuesWithCommunicationProviderUnavailable(t *testing.T) {
	databaseURL := os.Getenv("APGIC_JOURNEY_STORE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("journey integration database not configured")
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
		) VALUES ($1::uuid, 'COMMUNICATION_PROVIDER', 'unavailable-test-provider', 'DISABLED', $2)
	`, connectorID, "secretref://connector/"+connectorID); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	service, err := demand.NewConformanceServiceWithStores(func() time.Time { return now }, store, store)
	if err != nil {
		t.Fatal(err)
	}

	intent, err := service.CreateIntent("нужна помощь со сном")
	if err != nil {
		t.Fatalf("create intent with communication provider disabled: %v", err)
	}
	if _, err := service.ConfirmIntent(intent.ID, []string{"sleep"}, nil, nil); err != nil {
		t.Fatalf("confirm intent with communication provider disabled: %v", err)
	}

	slots, err := service.Slots("spec-lebedeva")
	if err != nil || len(slots) == 0 {
		t.Fatalf("slots with communication provider disabled: slots=%#v err=%v", slots, err)
	}
	hold, err := service.AcquireHold(intent.ID, slots[0].ID, intent.ClientIdentityID)
	if err != nil {
		t.Fatalf("booking hold with communication provider disabled: %v", err)
	}

	t.Cleanup(func() {
		result, cleanupErr := store.db.Exec(
			`UPDATE booking_holds
			    SET state = 'RELEASED',
			        updated_at = $2
			  WHERE id = $1::uuid
			    AND state = 'ACTIVE'`,
			hold.ID,
			time.Now().UTC(),
		)
		if cleanupErr != nil {
			t.Errorf("release CONN-001 booking hold: %v", cleanupErr)
			return
		}
		affected, cleanupErr := result.RowsAffected()
		if cleanupErr != nil {
			t.Errorf("read CONN-001 booking hold cleanup result: %v", cleanupErr)
			return
		}
		if affected != 1 {
			t.Errorf("released CONN-001 booking holds=%d want=1", affected)
		}
	})

	ledgerID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	economicEventRef := "conn001-core-proof/" + connectorID
	entry, err := store.AppendLedgerEntry(ledger.Entry{
		ID:                  ledgerID,
		DebitAccountRef:     "external-payer/" + intent.ClientIdentityID,
		CreditAccountRef:    "platform-evidence/conn001",
		AmountMinor:         1,
		Currency:            "RUB",
		ProviderEvidenceRef: "provider-evidence://conn001/non-custodial-proof",
		EconomicEventRef:    economicEventRef,
		CorrelationID:       "conn001-" + connectorID,
		OccurredAt:          now,
	})
	if err != nil {
		t.Fatalf("append ledger evidence with communication provider disabled: %v", err)
	}
	if entry.EconomicEventRef != economicEventRef {
		t.Fatalf("ledger economic event=%q want=%q", entry.EconomicEventRef, economicEventRef)
	}

	var identityCount int
	if err := store.db.QueryRow(
		`SELECT count(*) FROM identities WHERE id = $1::uuid`,
		intent.ClientIdentityID,
	).Scan(&identityCount); err != nil {
		t.Fatal(err)
	}
	if identityCount != 1 {
		t.Fatalf("canonical identity rows=%d want=1", identityCount)
	}

	var holdState string
	if err := store.db.QueryRow(
		`SELECT state FROM booking_holds WHERE id = $1::uuid`,
		hold.ID,
	).Scan(&holdState); err != nil {
		t.Fatal(err)
	}
	if holdState != "ACTIVE" {
		t.Fatalf("durable booking hold state=%q want ACTIVE", holdState)
	}
	if hold.BookingState != "HELD" {
		t.Fatalf("domain booking state=%q want HELD", hold.BookingState)
	}

	var ledgerCount int
	if err := store.db.QueryRow(
		`SELECT count(*) FROM ledger_entries WHERE economic_event_ref = $1`,
		economicEventRef,
	).Scan(&ledgerCount); err != nil {
		t.Fatal(err)
	}
	if ledgerCount != 1 {
		t.Fatalf("ledger entries for core proof=%d want=1", ledgerCount)
	}

	var connectorStatus string
	if err := store.db.QueryRow(
		`SELECT status FROM connector_instances WHERE id = $1::uuid`,
		connectorID,
	).Scan(&connectorStatus); err != nil {
		t.Fatal(err)
	}
	if connectorStatus != "DISABLED" {
		t.Fatalf("communication connector status=%q want DISABLED", connectorStatus)
	}
}
