package finance

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
	DisputedAmountMinor     int64    `json:"disputed_amount_minor"`
	RevenueAssurance        string   `json:"revenue_assurance"`
	RevenueReasonCode       string   `json:"revenue_reason_code"`
	ExecutionOwner          string   `json:"execution_owner"`
}
