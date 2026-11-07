package academy

// LearnerVideo intentionally has no cloud-provider identifiers or source URL.
// A valid entitlement gives metadata, not automatic media-byte permission.
type LearnerVideo struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Ordinal int    `json:"ordinal"`
}
