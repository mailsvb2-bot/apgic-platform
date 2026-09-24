package finance

import (
	"errors"
	"strings"
)

type ReconciliationOutcome string

const (
	ReconciliationMatched  ReconciliationOutcome = "MATCHED"
	ReconciliationMismatch ReconciliationOutcome = "MISMATCH"
)

type DiscrepancyStatus string

const (
	DiscrepancyOpen DiscrepancyStatus = "OPEN"
)

var (
	ErrInvalidReconciliation = errors.New("invalid reconciliation input")
	ErrDisputeInvalid        = errors.New("invalid dispute effect")
	ErrRevenueAssurance      = errors.New("revenue assurance blocked")
)

type ExpectedFinancialState struct {
	EconomicRef             string
	OriginalProviderID      string
	GrossMinor              int64
	ProviderFeeMinor        int64
	Currency                string
	LedgerEvidenceRef       string
	SettlementEvidenceRef   string
	PayoutEvidenceRef       string
}

type ProviderStatementFact struct {
	ProviderID          string
	EconomicRef         string
	GrossMinor          int64
	ProviderFeeMinor    int64
	Currency            string
	StatementEvidenceRef string
}

type ReconciliationDiscrepancy struct {
	EconomicRef          string
	ProviderID           string
	Kind                 string
	ExpectedGrossMinor   int64
	ProviderGrossMinor   int64
	ExpectedFeeMinor     int64
	ProviderFeeMinor     int64
	Currency             string
	Owner                string
	Status               DiscrepancyStatus
	EvidenceRefs         []string
}

type ReconciliationResult struct {
	Outcome     ReconciliationOutcome
	EconomicRef string
	ProviderID  string
	Discrepancy *ReconciliationDiscrepancy
}

func Reconcile(expected ExpectedFinancialState, statement ProviderStatementFact) (ReconciliationResult, error) {
	expected.Currency = strings.ToUpper(strings.TrimSpace(expected.Currency))
	statement.Currency = strings.ToUpper(strings.TrimSpace(statement.Currency))
	if strings.TrimSpace(expected.EconomicRef) == "" ||
		strings.TrimSpace(expected.OriginalProviderID) == "" ||
		expected.GrossMinor <= 0 ||
		expected.ProviderFeeMinor < 0 ||
		expected.ProviderFeeMinor > expected.GrossMinor ||
		len(expected.Currency) != 3 ||
		strings.TrimSpace(expected.LedgerEvidenceRef) == "" ||
		strings.TrimSpace(expected.SettlementEvidenceRef) == "" ||
		strings.TrimSpace(expected.PayoutEvidenceRef) == "" ||
		strings.TrimSpace(statement.ProviderID) == "" ||
		statement.EconomicRef != expected.EconomicRef ||
		statement.GrossMinor < 0 ||
		statement.ProviderFeeMinor < 0 ||
		len(statement.Currency) != 3 ||
		strings.TrimSpace(statement.StatementEvidenceRef) == "" {
		return ReconciliationResult{}, ErrInvalidReconciliation
	}
	if statement.ProviderID != expected.OriginalProviderID {
		return ReconciliationResult{}, ErrProviderAffinity
	}

	result := ReconciliationResult{
		Outcome: ReconciliationMatched,
		EconomicRef: expected.EconomicRef,
		ProviderID: expected.OriginalProviderID,
	}
	if statement.GrossMinor == expected.GrossMinor &&
		statement.ProviderFeeMinor == expected.ProviderFeeMinor &&
		statement.Currency == expected.Currency {
		return result, nil
	}

	result.Outcome = ReconciliationMismatch
	result.Discrepancy = &ReconciliationDiscrepancy{
		EconomicRef: expected.EconomicRef,
		ProviderID: expected.OriginalProviderID,
		Kind: "PROVIDER_LEDGER_MISMATCH",
		ExpectedGrossMinor: expected.GrossMinor,
		ProviderGrossMinor: statement.GrossMinor,
		ExpectedFeeMinor: expected.ProviderFeeMinor,
		ProviderFeeMinor: statement.ProviderFeeMinor,
		Currency: expected.Currency,
		Owner: "FINANCE_OPERATIONS",
		Status: DiscrepancyOpen,
		EvidenceRefs: []string{
			expected.LedgerEvidenceRef,
			expected.SettlementEvidenceRef,
			expected.PayoutEvidenceRef,
			statement.StatementEvidenceRef,
		},
	}
	return result, nil
}

type DisputeKind string

const (
	DisputeChargeback DisputeKind = "CHARGEBACK"
	DisputeRefund     DisputeKind = "REFUND"
	DisputeReversal   DisputeKind = "REVERSAL"
)

type DisputeEffect struct {
	EffectID             string
	EconomicRef          string
	OriginalProviderID   string
	ProviderID           string
	Kind                 DisputeKind
	AmountMinor          int64
	Currency             string
	ProviderEvidenceRef  string
	LedgerEvidenceRef    string
	AuditEvidenceRef     string
	ExecutionOwner       string
}

func NewDisputeEffect(effect DisputeEffect) (DisputeEffect, error) {
	effect.Currency = strings.ToUpper(strings.TrimSpace(effect.Currency))
	if strings.TrimSpace(effect.EffectID) == "" ||
		strings.TrimSpace(effect.EconomicRef) == "" ||
		strings.TrimSpace(effect.OriginalProviderID) == "" ||
		strings.TrimSpace(effect.ProviderID) == "" ||
		effect.AmountMinor <= 0 ||
		len(effect.Currency) != 3 ||
		strings.TrimSpace(effect.ProviderEvidenceRef) == "" ||
		strings.TrimSpace(effect.LedgerEvidenceRef) == "" ||
		strings.TrimSpace(effect.AuditEvidenceRef) == "" ||
		effect.ExecutionOwner != ExternalExecutionOwner {
		return DisputeEffect{}, ErrDisputeInvalid
	}
	switch effect.Kind {
	case DisputeChargeback, DisputeRefund, DisputeReversal:
	default:
		return DisputeEffect{}, ErrDisputeInvalid
	}
	if effect.ProviderID != effect.OriginalProviderID {
		return DisputeEffect{}, ErrProviderAffinity
	}
	return effect, nil
}

type RevenueEvidence struct {
	PaymentEvidenceRef      string
	LedgerEvidenceRef       string
	FulfillmentEvidenceRef  string
	CommissionEvidenceRef   string
	SettlementEvidenceRef   string
	PayoutEvidenceRef       string
	ReconciliationOutcome   ReconciliationOutcome
	DiscrepancyEvidenceRef  string
	APGICCustody            bool
}

type RevenueAssuranceResult struct {
	Outcome    string
	ReasonCode string
}

func EvaluateRevenueAssurance(evidence RevenueEvidence) (RevenueAssuranceResult, error) {
	required := []string{
		evidence.PaymentEvidenceRef,
		evidence.LedgerEvidenceRef,
		evidence.FulfillmentEvidenceRef,
		evidence.CommissionEvidenceRef,
		evidence.SettlementEvidenceRef,
		evidence.PayoutEvidenceRef,
	}
	for _, ref := range required {
		if strings.TrimSpace(ref) == "" {
			return RevenueAssuranceResult{Outcome: "BLOCK", ReasonCode: "REVENUE_EVIDENCE_MISSING"}, ErrRevenueAssurance
		}
	}
	if evidence.APGICCustody {
		return RevenueAssuranceResult{Outcome: "BLOCK", ReasonCode: "REVENUE_APGIC_CUSTODY_FORBIDDEN"}, ErrRevenueAssurance
	}
	switch evidence.ReconciliationOutcome {
	case ReconciliationMatched:
		return RevenueAssuranceResult{Outcome: "PASS", ReasonCode: "REVENUE_RECONCILED"}, nil
	case ReconciliationMismatch:
		if strings.TrimSpace(evidence.DiscrepancyEvidenceRef) == "" {
			return RevenueAssuranceResult{Outcome: "BLOCK", ReasonCode: "REVENUE_UNTYPED_MISMATCH"}, ErrRevenueAssurance
		}
		return RevenueAssuranceResult{Outcome: "PASS_WITH_EXCEPTION", ReasonCode: "REVENUE_TYPED_EXCEPTION"}, nil
	default:
		return RevenueAssuranceResult{Outcome: "BLOCK", ReasonCode: "REVENUE_RECONCILIATION_MISSING"}, ErrRevenueAssurance
	}
}
