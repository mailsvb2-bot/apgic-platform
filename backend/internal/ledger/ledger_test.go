package ledger

import (
	"reflect"
	"testing"
)

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

func TestLedgerRejectsMalformedCurrency(t *testing.T) {
	_, err := NewEntry(Entry{
		ID: "le-invalid", DebitAccountRef: "buyer", CreditAccountRef: "specialist",
		AmountMinor: 10000, Currency: "1$?", ProviderEvidenceRef: "provider:tx-invalid",
		CorrelationID: "corr-invalid",
	})
	if err != ErrInvalidEntry {
		t.Fatalf("expected malformed currency rejection, got %v", err)
	}
}

func TestReconcileReplaysExactMinorUnitBalances(t *testing.T) {
	entries := []Entry{
		{
			ID: "le-1", DebitAccountRef: "receivable", CreditAccountRef: "provider-clearing",
			AmountMinor: 10001, Currency: "rub", ProviderEvidenceRef: "provider:tx-1",
			CorrelationID: "corr-1",
		},
		{
			ID: "le-2", DebitAccountRef: "provider-clearing", CreditAccountRef: "specialist-payable",
			AmountMinor: 7000, Currency: "RUB", ProviderEvidenceRef: "provider:tx-2",
			CorrelationID: "corr-2",
		},
		{
			ID: "le-3", DebitAccountRef: "receivable", CreditAccountRef: "provider-clearing",
			AmountMinor: 250, Currency: "USD", ProviderEvidenceRef: "provider:tx-3",
			CorrelationID: "corr-3",
		},
	}

	got, err := Reconcile(entries)
	if err != nil {
		t.Fatal(err)
	}

	wantBalances := []AccountBalance{
		{AccountRef: "provider-clearing", Currency: "RUB", AmountMinor: 3001},
		{AccountRef: "receivable", Currency: "RUB", AmountMinor: -10001},
		{AccountRef: "specialist-payable", Currency: "RUB", AmountMinor: 7000},
		{AccountRef: "provider-clearing", Currency: "USD", AmountMinor: 250},
		{AccountRef: "receivable", Currency: "USD", AmountMinor: -250},
	}
	wantTotals := []CurrencyTotals{
		{Currency: "RUB", DebitMinor: 17001, CreditMinor: 17001},
		{Currency: "USD", DebitMinor: 250, CreditMinor: 250},
	}

	if got.EntryCount != 3 {
		t.Fatalf("entry count mismatch: %d", got.EntryCount)
	}
	if !reflect.DeepEqual(got.Balances, wantBalances) {
		t.Fatalf("balances mismatch:\n got: %#v\nwant: %#v", got.Balances, wantBalances)
	}
	if !reflect.DeepEqual(got.CurrencyTotals, wantTotals) {
		t.Fatalf("currency totals mismatch:\n got: %#v\nwant: %#v", got.CurrencyTotals, wantTotals)
	}
}

func TestReconcileIsIndependentOfInputOrder(t *testing.T) {
	first := Entry{
		ID: "le-a", DebitAccountRef: "buyer", CreditAccountRef: "provider",
		AmountMinor: 101, Currency: "RUB", ProviderEvidenceRef: "provider:a",
		CorrelationID: "corr-a",
	}
	second := Entry{
		ID: "le-b", DebitAccountRef: "provider", CreditAccountRef: "seller",
		AmountMinor: 80, Currency: "RUB", ProviderEvidenceRef: "provider:b",
		CorrelationID: "corr-b",
	}

	forward, err := Reconcile([]Entry{first, second})
	if err != nil {
		t.Fatal(err)
	}
	reversed, err := Reconcile([]Entry{second, first})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(forward, reversed) {
		t.Fatalf("reconciliation must be deterministic across input order:\nforward=%#v\nreversed=%#v", forward, reversed)
	}
}

func TestReconcileRejectsDuplicateEntryID(t *testing.T) {
	entry := Entry{
		ID: "le-dup", DebitAccountRef: "buyer", CreditAccountRef: "provider",
		AmountMinor: 100, Currency: "RUB", ProviderEvidenceRef: "provider:dup",
		CorrelationID: "corr-dup",
	}
	_, err := Reconcile([]Entry{entry, entry})
	if err != ErrDuplicateEntryID {
		t.Fatalf("expected duplicate entry rejection, got %v", err)
	}
}

func TestReconcileRejectsMinorUnitOverflow(t *testing.T) {
	entries := []Entry{
		{
			ID: "le-max", DebitAccountRef: "buyer-a", CreditAccountRef: "provider",
			AmountMinor: int64(^uint64(0) >> 1), Currency: "RUB",
			ProviderEvidenceRef: "provider:max", CorrelationID: "corr-max",
		},
		{
			ID: "le-overflow", DebitAccountRef: "buyer-b", CreditAccountRef: "provider",
			AmountMinor: 1, Currency: "RUB",
			ProviderEvidenceRef: "provider:overflow", CorrelationID: "corr-overflow",
		},
	}
	_, err := Reconcile(entries)
	if err != ErrReconcileOverflow {
		t.Fatalf("expected reconciliation overflow rejection, got %v", err)
	}
}
