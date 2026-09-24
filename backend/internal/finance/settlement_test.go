package finance

import (
	"errors"
	"reflect"
	"testing"
)

func commissionRule() CommissionRule {
	return CommissionRule{
		PolicyVersion:           "commission-r4-v1",
		DemandSource:            "MARKETPLACE_ORGANIC",
		ProductRef:              "consultation",
		Jurisdiction:            "RU",
		BasisPoints:             1000,
		RoundingMode:            "FLOOR_MINOR",
		PlatformFeeRecipientRef: "platform/apgic-fee",
	}
}

func TestCommissionRequiresExplicitSourceProductJurisdictionRule(t *testing.T) {
	ctx := CommissionContext{
		OrderID: "order-1", DemandSource: "MARKETPLACE_ORGANIC",
		ProductRef: "consultation", Jurisdiction: "RU",
		AmountMinor: 10001, Currency: "rub",
	}
	snapshot, err := CalculateCommission(ctx, []CommissionRule{commissionRule()})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CommissionMinor != 1000 || snapshot.RoundingMode != "FLOOR_MINOR" {
		t.Fatalf("unexpected commission snapshot: %#v", snapshot)
	}

	ctx.DemandSource = "PARTNER"
	if _, err := CalculateCommission(ctx, []CommissionRule{commissionRule()}); !errors.Is(err, ErrCommissionPolicyNotConfigured) {
		t.Fatalf("missing source rule must fail closed, got %v", err)
	}
}

func provider(id string) ProviderExecutionProfile {
	return ProviderExecutionProfile{
		ProviderID:     id,
		ExecutionOwner: ExternalExecutionOwner,
		Capabilities: []string{
			CapabilityMarketplaceSplit,
			CapabilitySettlementExecution,
			CapabilityPayoutExecution,
		},
	}
}

func settlementContext(t *testing.T) SettlementContext {
	t.Helper()
	commission, err := CalculateCommission(CommissionContext{
		OrderID: "order-1", DemandSource: "MARKETPLACE_ORGANIC",
		ProductRef: "consultation", Jurisdiction: "RU",
		AmountMinor: 10000, Currency: "RUB",
	}, []CommissionRule{commissionRule()})
	if err != nil {
		t.Fatal(err)
	}
	return SettlementContext{
		InstructionID:              "settlement-1",
		IdempotencyKey:             "settlement-key-1",
		OrderID:                    "order-1",
		PaymentAttemptID:           "payment-1",
		OriginalProviderID:         "provider-a",
		AmountMinor:                10000,
		Currency:                   "RUB",
		CaptureLedgerEntryRef:      "ledger/capture-1",
		CaptureProviderEvidenceRef: "provider-evidence/capture-1",
		CompletionEvidenceRef:      "consultation/evidence-ended-1",
		PayoutBeneficiaryRef:       "identity/specialist-1",
		Commission:                 commission,
	}
}

func TestSettlementAllocationsConvergeAndStayOnOriginalProvider(t *testing.T) {
	ctx := settlementContext(t)
	instruction, err := BuildSettlementInstruction(ctx, provider("provider-a"))
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, allocation := range instruction.Allocations {
		total += allocation.AmountMinor
	}
	if total != instruction.TotalAmountMinor || instruction.ExecutionOwner != ExternalExecutionOwner {
		t.Fatalf("invalid provider settlement instruction: %#v", instruction)
	}

	if _, err := BuildSettlementInstruction(ctx, provider("provider-b")); !errors.Is(err, ErrProviderAffinity) {
		t.Fatalf("cross-provider settlement must be rejected, got %v", err)
	}
}

func TestPayoutRequiresProviderCapabilityComplianceAndAffinity(t *testing.T) {
	ctx := settlementContext(t)
	settlement, err := BuildSettlementInstruction(ctx, provider("provider-a"))
	if err != nil {
		t.Fatal(err)
	}
	evidence := SettlementEvidence{
		SettlementInstructionID: settlement.InstructionID,
		ProviderID:              "provider-a",
		ProviderSettlementRef:   "provider/settlement-1",
		ProviderEvidenceRef:     "provider-evidence/settlement-1",
		LedgerEvidenceRef:       settlement.LedgerEvidenceRef,
	}
	eligibility := PayoutEligibility{
		DecisionID:               "payout-1",
		SettlementInstructionID:  settlement.InstructionID,
		OriginalProviderID:       "provider-a",
		BeneficiaryRef:           "identity/specialist-1",
		AmountMinor:              9000,
		Currency:                 "RUB",
		ProviderReportedEligible: true,
		ComplianceEvidenceRef:    "provider-evidence/compliance-1",
		IdempotencyKey:           "payout-key-1",
	}
	if _, err := BuildPayoutInstruction(eligibility, evidence, provider("provider-a")); err != nil {
		t.Fatal(err)
	}

	eligibility.ProviderReportedEligible = false
	if _, err := BuildPayoutInstruction(eligibility, evidence, provider("provider-a")); !errors.Is(err, ErrPayoutBlocked) {
		t.Fatalf("provider-ineligible payout must remain blocked, got %v", err)
	}

	eligibility.ProviderReportedEligible = true
	if _, err := BuildPayoutInstruction(eligibility, evidence, provider("provider-b")); !errors.Is(err, ErrProviderAffinity) {
		t.Fatalf("payout must preserve original provider affinity, got %v", err)
	}
}

func TestFinanceInstructionsContainNoAPGICCustodialBalance(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeOf(SettlementInstruction{}),
		reflect.TypeOf(PayoutInstruction{}),
	} {
		for _, forbidden := range []string{
			"Balance", "WalletBalance", "CustodialAccount", "APGICBalance", "BridgeBalance",
		} {
			if _, ok := typ.FieldByName(forbidden); ok {
				t.Fatalf("%s exposes forbidden custodial field %s", typ.Name(), forbidden)
			}
		}
	}
}
