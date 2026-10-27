package privacy

import "time"

// AccountDeletionReceipt describes a registered request, not completed erasure.
type AccountDeletionReceipt struct {
	ID         string    `json:"id"`
	State      string    `json:"state"`
	Source     string    `json:"source"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Idempotent bool      `json:"idempotent"`
	Notice     string    `json:"notice"`
	RetainedReason string `json:"retained_reason,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	PendingProviders int `json:"pending_providers"`
	ReviewCode string `json:"review_code,omitempty"`
}
