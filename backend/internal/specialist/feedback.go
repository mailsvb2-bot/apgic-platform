package specialist

import "errors"

var (
	ErrFeedbackInvalid = errors.New("verified specialist feedback rating or booking id invalid")
	ErrFeedbackNotEligible = errors.New("client has no completed live booking for feedback")
	ErrFeedbackDuplicate = errors.New("verified feedback already submitted for this booking")
	ErrFeedbackNotFound = errors.New("verified feedback not found")
)

type FeedbackSubmission struct {
	ID    string `json:"id"`
	State string `json:"state"`
	Notice string `json:"notice"`
}

// FeedbackStore is the canonical evidence-based reputation owner.
// Only signed booking-client identity can submit; only a trusted reviewer
// can approve. Caller-provided specialist identity is never accepted.
type FeedbackStore interface {
	SubmitVerifiedFeedback(clientIdentityID, bookingID string, rating int) (FeedbackSubmission,error)
	ReviewVerifiedFeedback(feedbackID string, approved bool, reviewerRef string) error
}
