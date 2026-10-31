package academy

import "time"

// Appeal is owned by the graded learner and may be resolved only by an
// independently authorized reviewer, not by the course author or learner.
type Appeal struct {
	ID          string     `json:"id"`
	AttemptID   string     `json:"attempt_id"`
	State       string     `json:"state"`
	Reason      string     `json:"reason"`
	SubmittedAt time.Time  `json:"submitted_at"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
}
