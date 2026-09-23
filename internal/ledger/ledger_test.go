package ledger

import "testing"

func TestLedgerBatchBalancesPerCurrency(t *testing.T) {
	entries := []Entry{
		{ID: "1", EvidenceID: "provider-evidence-1", Account: "receivable", Currency: "RUB", Side: Debit, AmountMinor: 10000},
		{ID: "2", EvidenceID: "provider-evidence-1", Account: "provider-clearing", Currency: "RUB", Side: Credit, AmountMinor: 10000},
	}
	if err := ValidateBalanced(entries); err != nil {
		t.Fatal(err)
	}
	entries[1].AmountMinor = 9999
	if err := ValidateBalanced(entries); err == nil {
		t.Fatal("unbalanced ledger batch must be rejected")
	}
}
