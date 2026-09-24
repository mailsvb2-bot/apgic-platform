package finance

import (
	"errors"
	"testing"
)

func expectedFinancialState() ExpectedFinancialState {
	return ExpectedFinancialState{
		EconomicRef:           "order/order-1",
		OriginalProviderID:    "provider-a",
		GrossMinor:            10000,
		ProviderFeeMinor:      200,
		Currency:              "RUB",
		LedgerEvidenceRef:     "ledger/capture-1",
		SettlementEvidenceRef: "provider/settlement-1",
		PayoutEvidenceRef:     "provider/payout-1",
	}
}

func TestReconciliationClosesExactMatchAndTypesMismatch(t *testing.T) {
	expected := expectedFinancialState()
	statement := ProviderStatementFact{
		ProviderID:           "provider-a",
		EconomicRef:          expected.EconomicRef,
		GrossMinor:           10000,
		ProviderFeeMinor:     200,
		Currency:             "RUB",
		StatementEvidenceRef: "provider-statement/1",
	}
	result, err := Reconcile(expected, statement)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != ReconciliationMatched || result.Discrepancy != nil {
		t.Fatalf("exact match result=%#v", result)
	}

	statement.ProviderFeeMinor = 250
	result, err = Reconcile(expected, statement)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != ReconciliationMismatch ||
		result.Discrepancy == nil ||
		result.Discrepancy.Owner != "FINANCE_OPERATIONS" ||
		result.Discrepancy.Status != DiscrepancyOpen ||
		len(result.Discrepancy.EvidenceRefs) != 4 {
		t.Fatalf("mismatch must create typed discrepancy: %#v", result)
	}
}

func TestReconciliationCannotMoveToAnotherProvider(t *testing.T) {
	expected := expectedFinancialState()
	statement := ProviderStatementFact{
		ProviderID:           "provider-b",
		EconomicRef:          expected.EconomicRef,
		GrossMinor:           10000,
		ProviderFeeMinor:     200,
		Currency:             "RUB",
		StatementEvidenceRef: "provider-statement/b",
	}
	if _, err := Reconcile(expected, statement); !errors.Is(err, ErrProviderAffinity) {
		t.Fatalf("cross-provider reconciliation must fail, got %v", err)
	}
}

func TestDisputeEffectIsProviderExecutedAndAppendOnlyShaped(t *testing.T) {
	effect, err := NewDisputeEffect(DisputeEffect{
		EffectID:            "dispute-effect-1",
		EconomicRef:         "order/order-1",
		OriginalProviderID:  "provider-a",
		ProviderID:          "provider-a",
		Kind:                DisputeChargeback,
		AmountMinor:         10000,
		Currency:            "rub",
		ProviderEvidenceRef: "provider-evidence/chargeback-1",
		LedgerEvidenceRef:   "ledger/reversal-1",
		AuditEvidenceRef:    "audit/dispute-1",
		ExecutionOwner:      ExternalExecutionOwner,
	})
	if err != nil {
		t.Fatal(err)
	}
	if effect.Currency != "RUB" {
		t.Fatalf("currency=%s", effect.Currency)
	}

	effect.ProviderID = "provider-b"
	if _, err := NewDisputeEffect(effect); !errors.Is(err, ErrProviderAffinity) {
		t.Fatalf("dispute must preserve provider affinity, got %v", err)
	}
}

func TestRevenueAssuranceRequiresCompleteChainOrTypedException(t *testing.T) {
	base := RevenueEvidence{
		PaymentEvidenceRef:     "payment/capture-1",
		LedgerEvidenceRef:      "ledger/capture-1",
		FulfillmentEvidenceRef: "consultation/end-1",
		CommissionEvidenceRef:  "commission/1",
		SettlementEvidenceRef:  "settlement/1",
		PayoutEvidenceRef:      "payout/1",
		ReconciliationOutcome:  ReconciliationMatched,
	}
	result, err := EvaluateRevenueAssurance(base)
	if err != nil || result.Outcome != "PASS" {
		t.Fatalf("matched chain result=%#v err=%v", result, err)
	}

	base.ReconciliationOutcome = ReconciliationMismatch
	if _, err := EvaluateRevenueAssurance(base); !errors.Is(err, ErrRevenueAssurance) {
		t.Fatalf("untyped mismatch must block, got %v", err)
	}
	base.DiscrepancyEvidenceRef = "recon-discrepancy/1"
	result, err = EvaluateRevenueAssurance(base)
	if err != nil || result.Outcome != "PASS_WITH_EXCEPTION" {
		t.Fatalf("typed discrepancy result=%#v err=%v", result, err)
	}

	base.APGICCustody = true
	if _, err := EvaluateRevenueAssurance(base); !errors.Is(err, ErrRevenueAssurance) {
		t.Fatalf("APGIC custody must block release, got %v", err)
	}
}
