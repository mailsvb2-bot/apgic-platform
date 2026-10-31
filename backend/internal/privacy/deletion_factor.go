package privacy

import "time"

// DeletionFactorStatus contains no OTP seed or authentication secret.
type DeletionFactorStatus struct {
	Enrolled bool      `json:"enrolled"`
	ReadyAt  time.Time `json:"ready_at,omitempty"`
}
