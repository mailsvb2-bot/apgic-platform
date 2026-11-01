package academy

// AuthorReport is a privacy-safe, author-scoped read model over the canonical
// Academy and Commerce records. It does not constitute a payout or net revenue.
type AuthorReport struct {
	Courses       []CourseReport          `json:"courses"`
	GrossCaptures []CapturedGrossEvidence `json:"gross_captures"`
	RevenueState  string                  `json:"revenue_state"`
}

type CourseReport struct {
	CourseID           string `json:"course_id"`
	Title              string `json:"title"`
	Enrollments        int64  `json:"enrollments"`
	Completed          int64  `json:"completed"`
	ActiveAccess       int64  `json:"active_access"`
	PendingAssessments int64  `json:"pending_assessments"`
	PassedAssessments  int64  `json:"passed_assessments"`
}

// CapturedGrossEvidence is sourced from canonical captured Store transactions
// with a matching immutable order.product_id, not from client assertions.
// It is gross payment volume, NOT net revenue, author royalties or a payout.
// ActiveCount/ActiveGrossMinor are the historical captured orders linked
// to an ACTIVE/COMPLETED Academy enrollment whose canonical entitlement is
// still valid at read time. A paid but not yet enrolled learner, or a revoked
// enrollment with an otherwise active entitlement, is not counted here.
// They are NOT refund amounts, settled receipts, commission-adjusted income
// or payout.
type CapturedGrossEvidence struct {
	CourseID           string `json:"course_id"`
	Currency           string `json:"currency"`
	CapturedCount      int64  `json:"captured_count"`
	CapturedGrossMinor int64  `json:"captured_gross_minor"`
	ActiveCount        int64  `json:"active_count"`
	ActiveGrossMinor   int64  `json:"active_gross_minor"`
}

const RevenueGrossOnly = "GROSS_CAPTURED_ONLY_NO_REFUND_COMMISSION_OR_PAYOUT_RECONCILIATION"
