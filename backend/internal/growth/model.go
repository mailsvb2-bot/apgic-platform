package growth

import "time"

// Link's economic source and owner are assigned by PostgreSQL after verifying
// an active qualification and published product. No browser field sets them.
type Link struct {
	ID            string    `json:"id"`
	OfferID       string    `json:"offer_id"`
	TopicID       string    `json:"topic_id"`
	Source        string    `json:"source"`
	ExpiresAt     time.Time `json:"expires_at"`
	PolicyVersion string    `json:"policy_version"`
	State         string    `json:"state"`
	Token         string    `json:"token,omitempty"`
	Path          string    `json:"path,omitempty"`
}

type Touch struct {
	ID             string    `json:"id"`
	LinkID         string    `json:"link_id"`
	Source         string    `json:"source"`
	OfferID        string    `json:"offer_id"`
	TopicID        string    `json:"topic_id"`
	SpecialistName string    `json:"specialist_name"`
	RecordedAt     time.Time `json:"recorded_at"`
	Idempotent     bool      `json:"idempotent"`
}

// Aggregate report intentionally excludes free-text HelpIntent, individual
// client identifiers, consultation data, and implied financial outcomes.
type Report struct {
	LinkID           string `json:"link_id"`
	OfferID          string `json:"offer_id"`
	Source           string `json:"source"`
	State            string `json:"state"`
	Touches          int64  `json:"touches"`
	HelpIntents      int64  `json:"help_intents"`
	ConfirmedIntents int64  `json:"confirmed_intents"`
}

type Store interface {
	IssueSpecialistLink(ownerIdentityID, offerID string, expiresAt time.Time) (Link, error)
	CaptureTouch(clientIdentityID, linkID string, at time.Time) (Touch, error)
	RevokeSpecialistLink(ownerIdentityID, linkID string) error
	SpecialistGrowthReport(ownerIdentityID string) ([]Report, error)
}
