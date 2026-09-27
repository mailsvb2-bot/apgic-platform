package runtimepostgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/ledger"
)

func TestRequiresDatabaseOnlyForRuntimeEnvironments(t *testing.T) {
	for _, value := range []string{"STAGING", "staging", "PRODUCTION", " production "} {
		if !RequiresDatabase(value) {
			t.Fatalf("%q must require database", value)
		}
	}
	for _, value := range []string{"", "CI", "DEV", "LOCAL"} {
		if RequiresDatabase(value) {
			t.Fatalf("%q must not require database", value)
		}
	}
}

func TestRequiredCanonicalTablesAreStable(t *testing.T) {
	want := map[string]bool{
		"identities": true, "outbox_events": true, "audit_records": true,
		"ledger_entries": true, "booking_slots": true, "booking_holds": true,
		"bookings": true,
	}
	if len(requiredTables) != len(want) {
		t.Fatalf("required tables = %v", requiredTables)
	}
	for _, table := range requiredTables {
		if !want[table] {
			t.Fatalf("unexpected required table %q", table)
		}
	}
}

func TestLedgerStorePersistsAndReplays(t *testing.T) {
	databaseURL := os.Getenv("APGIC_LEDGER_STORE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("integration database not configured")
	}
	store, err := Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	id, err := ledger.NewPersistentID()
	if err != nil {
		t.Fatal(err)
	}
	entry := ledger.Entry{
		ID:                  id,
		DebitAccountRef:     "external-provider/integration/settlement",
		CreditAccountRef:    "identity/integration-specialist",
		AmountMinor:         4321,
		Currency:            "rub",
		ProviderEvidenceRef: "integration/provider-event-1",
		EconomicEventRef:    "integration/order-1",
		CorrelationID:       "integration/correlation-1",
		OccurredAt:          time.Now().UTC().Truncate(time.Microsecond),
	}
	if err := store.AppendLedgerEntry(entry); err != nil {
		t.Fatal(err)
	}
	entries, err := store.LedgerEntries()
	if err != nil {
		t.Fatal(err)
	}
	var found *ledger.Entry
	for i := range entries {
		if entries[i].ID == id {
			found = &entries[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("persisted ledger entry %s not replayed", id)
	}
	if found.AmountMinor != 4321 || found.Currency != "RUB" ||
		found.ProviderEvidenceRef != entry.ProviderEvidenceRef ||
		found.EconomicEventRef != entry.EconomicEventRef {
		t.Fatalf("persisted ledger entry mismatch: %#v", found)
	}
	reconciliation, err := ledger.Reconcile(entries)
	if err != nil {
		t.Fatal(err)
	}
	if reconciliation.EntryCount != len(entries) {
		t.Fatalf("reconciliation entry count=%d rows=%d", reconciliation.EntryCount, len(entries))
	}

	retryID, err := ledger.NewPersistentID()
	if err != nil {
		t.Fatal(err)
	}
	retry := entry
	retry.ID = retryID
	retry.OccurredAt = entry.OccurredAt.Add(time.Second)
	if err := store.AppendLedgerEntry(retry); err != nil {
		t.Fatalf("idempotent retry must succeed: %v", err)
	}
	entries, err = store.LedgerEntries()
	if err != nil {
		t.Fatal(err)
	}
	eventCount := 0
	for _, candidate := range entries {
		if candidate.EconomicEventRef == entry.EconomicEventRef {
			eventCount++
		}
	}
	if eventCount != 1 {
		t.Fatalf("idempotent retry created %d rows for %s", eventCount, entry.EconomicEventRef)
	}

	collision := retry
	collision.AmountMinor++
	if err := store.AppendLedgerEntry(collision); err == nil {
		t.Fatal("same economic event with different financial effect must fail")
	}
}
