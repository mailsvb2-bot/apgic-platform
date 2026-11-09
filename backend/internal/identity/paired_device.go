package identity

import "time"

// PairedDevice describes only an authorized, revocable browser session.
// It never contains cookie values, cryptographic material or device fingerprinting.
type PairedDevice struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}
