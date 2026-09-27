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

func TestRequiredCanonicalTablesAndIndexesAreStable(t *testing.T) {
	wantTables := map[string]bool{
		"identities": true, "outbox_events": true, "audit_records": true,
		"ledger_entries": true, "booking_slots": true, "booking_holds": true,
		"bookings": true,
	}
	if len(requiredTables) != len(wantTables) {
		t.Fatalf("required tables = %v", requiredTables)
	}
	for _, table := range requiredTables {
		if !wantTables[table] {
			t.Fatalf("unexpected required table %q", table)
		}
	}
	if len(requiredIndexes) != 1 || requiredIndexes[0] != "ledger_entries_economic_event_ref_unique" {
		t.Fatalf("required indexes = %v", requiredIndexes)
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
	persisted, err := store.AppendLedgerEntry(entry)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.ID != id {
		t.Fatalf("insert returned persisted id %q want %q", persisted.ID, id)
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
	persistedRetry, err := store.AppendLedgerEntry(retry)
	if err != nil {
		t.Fatalf("idempotent retry must succeed: %v", err)
	}
	if persistedRetry.ID != id {
		t.Fatalf("idempotent retry returned non-persisted id %q want %q", persistedRetry.ID, id)
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
	if _, err := store.AppendLedgerEntry(collision); err == nil {
		t.Fatal("same economic event with different financial effect must fail")
	}

	if _, err := store.db.Exec("DROP INDEX ledger_entries_economic_event_ref_unique"); err != nil {
		t.Fatal(err)
	}
	indexRestored := false
	defer func() {
		if !indexRestored {
			_, _ = store.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS ledger_entries_economic_event_ref_unique
				ON ledger_entries (economic_event_ref)
				WHERE economic_event_ref IS NOT NULL`)
		}
	}()
	if err := store.Ready(context.Background()); err == nil {
		t.Fatal("readiness must fail when ledger idempotency index is missing")
	}
	if _, err := store.db.Exec(`CREATE UNIQUE INDEX ledger_entries_economic_event_ref_unique
		ON ledger_entries (economic_event_ref)
		WHERE economic_event_ref IS NOT NULL`); err != nil {
		t.Fatal(err)
	}
	indexRestored = true
	if err := store.Ready(context.Background()); err != nil {
		t.Fatalf("readiness must recover after idempotency index restore: %v", err)
	}
}
