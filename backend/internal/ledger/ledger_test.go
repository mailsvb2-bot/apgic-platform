package ledger

import "testing"

func TestLedgerEntryRequiresProviderEvidence(t *testing.T) {
	_, err := NewEntry(Entry{
		ID: "le-1", DebitAccountRef: "buyer", CreditAccountRef: "specialist",
		AmountMinor: 10000, Currency: "rub", CorrelationID: "corr-1",
	})
	if err != ErrInvalidEntry {
		t.Fatalf("expected provider evidence guard, got %v", err)
	}
}

func TestLedgerUsesMinorUnitsAndNormalizesCurrency(t *testing.T) {
	e, err := NewEntry(Entry{
		ID: "le-1", DebitAccountRef: "buyer", CreditAccountRef: "specialist",
		AmountMinor: 10000, Currency: "rub", ProviderEvidenceRef: "provider:tx-1",
		CorrelationID: "corr-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if e.Currency != "RUB" {
		t.Fatalf("currency must normalize to ISO-style uppercase code: %s", e.Currency)
	}
}
