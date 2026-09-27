package demand

import (
	"errors"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/ledger"
)

type fakeLedgerStore struct {
	entries   []ledger.Entry
	appendErr error
	readErr   error
}

func (s *fakeLedgerStore) AppendLedgerEntry(entry ledger.Entry) error {
	if s.appendErr != nil {
		return s.appendErr
	}
	s.entries = append(s.entries, entry)
	return nil
}

func (s *fakeLedgerStore) LedgerEntries() ([]ledger.Entry, error) {
	if s.readErr != nil {
		return nil, s.readErr
	}
	return append([]ledger.Entry(nil), s.entries...), nil
}

func TestLedgerStatePreviewCanonicalizesBeforeCommit(t *testing.T) {
	state := ledgerState{}
	prospective, canonical, reconciliation, err := state.preview(ledger.Entry{
		ID:                  "led-preview-1",
		DebitAccountRef:     "external-provider/provider-a/settlement",
		CreditAccountRef:    "identity-specialist-1",
		AmountMinor:         101,
		Currency:            "rub",
		ProviderEvidenceRef: "provider-a/event-1",
		CorrelationID:       "corr-preview-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if reconciliation.EntryCount != 1 || canonical.Currency != "RUB" {
		t.Fatalf("reconciliation=%#v canonical=%#v", reconciliation, canonical)
	}
	state.commit(prospective)
	if len(state.entries) != 1 || state.entries[0].Currency != "RUB" {
		t.Fatalf("ledger state must store canonical entry: %#v", state.entries)
	}
}

func TestLedgerStateRejectsInvalidEntryWithoutMutation(t *testing.T) {
	state := ledgerState{}
	if _, _, _, err := state.preview(ledger.Entry{
		ID:               "led-invalid",
		DebitAccountRef:  "buyer",
		CreditAccountRef: "seller",
		AmountMinor:      100,
		Currency:         "RUB",
		CorrelationID:    "corr-invalid",
	}); err == nil {
		t.Fatal("preview must reject missing provider evidence")
	}
	if len(state.entries) != 0 {
		t.Fatalf("invalid preview mutated ledger state: %#v", state.entries)
	}
}

func TestLedgerReconciliationRebuildsFromDurableStore(t *testing.T) {
	store := &fakeLedgerStore{entries: []ledger.Entry{
		{
			ID:                  "11111111-1111-4111-8111-111111111111",
			DebitAccountRef:     "external-provider/bank/settlement",
			CreditAccountRef:    "identity-specialist-1",
			AmountMinor:         4200,
			Currency:            "RUB",
			ProviderEvidenceRef: "bank/event-1",
			CorrelationID:       "order-1",
		},
	}}

	first := NewConformanceServiceWithLedgerStore(nil, store)
	got, err := first.LedgerReconciliation()
	if err != nil {
		t.Fatal(err)
	}
	if got.EntryCount != 1 {
		t.Fatalf("first reconciliation = %#v", got)
	}

	restarted := NewConformanceServiceWithLedgerStore(nil, store)
	got, err = restarted.LedgerReconciliation()
	if err != nil {
		t.Fatal(err)
	}
	if got.EntryCount != 1 || got.CurrencyTotals[0].DebitMinor != 4200 {
		t.Fatalf("restart must replay durable ledger: %#v", got)
	}
}

func TestDurableLedgerReadFailureFailsClosed(t *testing.T) {
	store := &fakeLedgerStore{readErr: errors.New("db unavailable")}
	service := NewConformanceServiceWithLedgerStore(nil, store)
	if _, err := service.LedgerReconciliation(); err == nil {
		t.Fatal("ledger reconciliation must fail closed when durable store cannot be read")
	}
}
