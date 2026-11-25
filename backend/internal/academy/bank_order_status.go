package academy

// BankOrderStatus is a read-only canonical Commerce projection for a signed
// buyer. It carries no PSP secrets or payment redirect and cannot grant rights.
type BankOrderStatus struct {
	OrderID                string `json:"order_id"`
	CourseID               string `json:"course_id"`
	AmountMinor            int64  `json:"amount_minor"`
	Currency               string `json:"currency"`
	PaymentState           string `json:"payment_state"`
	HasProviderReference   bool   `json:"provider_reference_recorded"`
	EntitlementActive      bool   `json:"entitlement_active"`
	EntitlementID          string `json:"entitlement_id,omitempty"`
	RefundID                string `json:"refund_id,omitempty"`
	RefundState             string `json:"refund_state,omitempty"`
	RequiresReconciliation bool   `json:"requires_reconciliation"`
}
