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
}
