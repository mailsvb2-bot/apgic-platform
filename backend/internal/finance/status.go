package finance

import "time"

// Owner-facing, provider-neutral read model for R4 financial status. APGIC
// only reports provider-executed facts; it never exposes or holds a balance.

type EvidenceDecision struct {
	EvidenceID string `json:"evidence_id"`
	Accepted   bool   `json:"accepted"`
	ReasonCode string `json:"reason_code"`
}

type OrderStatus struct {
	OrderID                 string   `json:"order_id"`
	Currency                string   `json:"currency"`
	TotalAmountMinor        int64    `json:"total_amount_minor"`
	CommissionPolicyVersion string   `json:"commission_policy_version,omitempty"`
	PlatformFeeMinor        *int64   `json:"platform_fee_minor,omitempty"`
	BeneficiaryAmountMinor  *int64   `json:"beneficiary_amount_minor,omitempty"`
	SettlementStatus        string   `json:"settlement_status"`
	SettlementHoldReasons   []string `json:"settlement_hold_reasons,omitempty"`
	PayoutStatus            string   `json:"payout_status"`
	PayoutReasonCode        string   `json:"payout_reason_code,omitempty"`
	ReconciliationOutcome   string   `json:"reconciliation_outcome"`
	DiscrepancyStatus       string   `json:"discrepancy_status"`
	DiscrepancyResolution   string   `json:"discrepancy_resolution_kind,omitempty"`
	DisputedAmountMinor     int64    `json:"disputed_amount_minor"`
	RevenueAssurance        string   `json:"revenue_assurance"`
	RevenueReasonCode       string   `json:"revenue_reason_code"`
	ExecutionOwner          string   `json:"execution_owner"`
}

// Discrepancy resolution kinds (append-only, typed). Mirrors migration 000040.
const (
	ResolutionProviderCorrectionConfirmed = "PROVIDER_CORRECTION_CONFIRMED"
	ResolutionSupersededByMatched         = "SUPERSEDED_BY_MATCHED_RECONCILIATION"
	ResolutionOperationalException        = "OPERATIONAL_EXCEPTION_ACCEPTED"

	CapabilityDiscrepancyResolve = "FINANCE_DISCREPANCY_RESOLVE"
)

func ValidResolutionKind(kind string) bool {
	switch kind {
	case ResolutionProviderCorrectionConfirmed, ResolutionSupersededByMatched, ResolutionOperationalException:
		return true
	}
	return false
}

// DiscrepancyResolutionRequest is a finance-operator decision. The operator,
// step-up session and audit reference are bound by the API, never by the body.
type DiscrepancyResolutionRequest struct {
	DiscrepancyID       string    `json:"-"`
	IdempotencyKey      string    `json:"idempotency_key"`
	Kind                string    `json:"kind"`
	ProviderEvidenceID  string    `json:"provider_evidence_id,omitempty"`
	SupersedingResultID string    `json:"superseding_result_id,omitempty"`
	Reason              string    `json:"reason"`
	EvidenceRefs        []string  `json:"evidence_refs"`
	OperatorID          string    `json:"-"`
	OperatorGrantID     string    `json:"-"`
	StepUpSessionRef    string    `json:"-"`
	AuditRef            string    `json:"-"`
	ResolvedAt          time.Time `json:"-"`
}

type DiscrepancyResolution struct {
	ResolutionID        string    `json:"resolution_id"`
	DiscrepancyID       string    `json:"discrepancy_id"`
	Kind                string    `json:"kind"`
	ResolvedBy          string    `json:"resolved_by"`
	ResolvedAt          time.Time `json:"resolved_at"`
	ProviderEvidenceID  string    `json:"provider_evidence_id,omitempty"`
	SupersedingResultID string    `json:"superseding_result_id,omitempty"`
	Replayed            bool      `json:"replayed"`
}

type OpenDiscrepancy struct {
	DiscrepancyID           string    `json:"discrepancy_id"`
	Kind                    string    `json:"kind"`
	SettlementInstructionID string    `json:"settlement_instruction_id"`
	ProviderInstanceID      string    `json:"provider_instance_id"`
	ExpectedFeeMinor        int64     `json:"expected_fee_minor"`
	ProviderFeeMinor        int64     `json:"provider_fee_minor"`
	ExpectedGrossMinor      int64     `json:"expected_gross_minor"`
	ProviderGrossMinor      int64     `json:"provider_gross_minor"`
	Currency                string    `json:"currency"`
	EvidenceRefs            []string  `json:"evidence_refs"`
	CreatedAt               time.Time `json:"created_at"`
}
