package demand

import (
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/ledger"
)

func TestLedgerStatePreviewCanonicalizesBeforeCommit(t *testing.T) {
	state := ledgerState{}
	prospective, reconciliation, err := state.preview(ledger.Entry{
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
	if reconciliation.EntryCount != 1 {
		t.Fatalf("reconciliation = %#v", reconciliation)
	}
	state.commit(prospective)
	if len(state.entries) != 1 || state.entries[0].Currency != "RUB" {
		t.Fatalf("ledger state must store canonical entry: %#v", state.entries)
	}
}

func TestLedgerStateRejectsInvalidEntryWithoutMutation(t *testing.T) {
	state := ledgerState{}
	if _, _, err := state.preview(ledger.Entry{
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
