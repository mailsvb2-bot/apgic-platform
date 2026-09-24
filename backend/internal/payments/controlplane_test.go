package payments

import (
	"errors"
	"reflect"
	"testing"
)

func certifiedProvider(id ProviderID, priority int) ProviderInstance {
	return ProviderInstance{
		Manifest: ProviderManifest{
			Version:                   "manifest-v1",
			ProviderID:                id,
			Certified:                 true,
			CertificationEvidenceRefs: []string{"evidence/sandbox"},
			ExecutionOwner:            ExternalExecutionOwner,
			Methods:                   []MethodCode{MethodBankCard, MethodSBP},
			Rails:                     []RailCode{RailAPGICPaymentProvider},
			Currencies:                []string{"RUB"},
			Jurisdictions:             []string{"RU"},
			RegulatedCapabilities:     []string{"ACQUIRING", "REFUND", "WEBHOOK"},
		},
		Status:   ProviderActive,
		Health:   HealthHealthy,
		Priority: priority,
	}
}

func TestSelectProviderIsDeterministicAcrossMultipleActiveProviders(t *testing.T) {
	providers := []ProviderInstance{
		certifiedProvider("provider-b", 20),
		certifiedProvider("provider-a", 10),
	}
	policy := RoutingPolicy{
		Version:          "routing-v7",
		OrderedProviders: []ProviderID{"provider-a", "provider-b"},
	}
	tx := TransactionContext{
		OrderID: "order-1", AmountMinor: 10000, Currency: "RUB",
		Jurisdiction: "RU", Method: MethodSBP, Rail: RailAPGICPaymentProvider,
	}

	first, err := SelectProvider(tx, providers, policy)
	if err != nil {
		t.Fatal(err)
	}
	second, err := SelectProvider(tx, providers, policy)
	if err != nil {
		t.Fatal(err)
	}
	if first.ProviderID != "provider-a" || !reflect.DeepEqual(first, second) {
		t.Fatalf("routing must be deterministic and policy ordered: %#v %#v", first, second)
	}
	if len(first.Candidates) != 1 || !first.Candidates[0].Eligible {
		t.Fatalf("selected provider must carry reproducible candidate evidence: %#v", first.Candidates)
	}
}

func TestRoutingFailsClosedForUnknownJurisdiction(t *testing.T) {
	tx := TransactionContext{
		OrderID: "order-1", AmountMinor: 10000, Currency: "RUB",
		Jurisdiction: "UNKNOWN", Method: MethodBankCard, Rail: RailAPGICPaymentProvider,
	}
	decision, err := SelectProvider(
		tx,
		[]ProviderInstance{certifiedProvider("provider-a", 10)},
		RoutingPolicy{Version: "routing-v7", OrderedProviders: []ProviderID{"provider-a"}},
	)
	if !errors.Is(err, ErrNoEligibleProvider) {
		t.Fatalf("unknown jurisdiction must fail closed, got %v", err)
	}
	if decision.Candidates[0].ReasonCode != ReasonJurisdictionUnsupported {
		t.Fatalf("unexpected exclusion reason: %#v", decision.Candidates)
	}
}

func TestManifestRequiresExternalExecutionAndCertificationEvidence(t *testing.T) {
	provider := certifiedProvider("provider-a", 10)
	provider.Manifest.ExecutionOwner = "APGIC"
	if err := provider.Manifest.Validate(); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("APGIC-owned money execution must be rejected, got %v", err)
	}

	provider = certifiedProvider("provider-a", 10)
	provider.Manifest.CertificationEvidenceRefs = nil
	if err := provider.Manifest.Validate(); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("certified provider without evidence must be rejected, got %v", err)
	}
}

func TestAmbiguousOrInFlightAttemptCannotBlindFallback(t *testing.T) {
	for _, state := range []AttemptState{AttemptSent, AttemptPending, AttemptAmbiguous, AttemptSucceeded} {
		if CanFallback(state) {
			t.Fatalf("%s must not allow cross-provider fallback", state)
		}
	}
	if !CanFallback(AttemptCreated) || !CanFallback(AttemptFailedTerminal) {
		t.Fatal("pre-send and proven terminal failure must remain retryable")
	}
}

func TestEligibleMethodCatalogIncludesOnlyEligibleProviders(t *testing.T) {
	good := certifiedProvider("provider-a", 10)
	bad := certifiedProvider("provider-b", 20)
	bad.Manifest.Jurisdictions = []string{"KZ"}

	methods := EligibleMethods(
		CatalogContext{Currency: "RUB", Jurisdiction: "RU", Rail: RailAPGICPaymentProvider},
		[]ProviderInstance{bad, good},
	)
	if !reflect.DeepEqual(methods, []MethodCode{MethodBankCard, MethodSBP}) {
		t.Fatalf("unexpected eligible method catalog: %#v", methods)
	}
}

func TestPaymentInstructionHasNoCustodialBalanceField(t *testing.T) {
	typ := reflect.TypeOf(PaymentInstruction{})
	for _, forbidden := range []string{"Balance", "WalletBalance", "CustodialAccount", "PayoutBalance"} {
		if _, ok := typ.FieldByName(forbidden); ok {
			t.Fatalf("custodial field %s must not exist in APGIC payment instruction", forbidden)
		}
	}
}
