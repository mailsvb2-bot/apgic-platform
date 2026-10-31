package academy

import "time"

// Cohort is an educational grouping, not a payment, booking, or identity owner.
// Its capacity and membership are enforced transactionally by PostgreSQL.
type Cohort struct {
	ID                 string     `json:"id"`
	VersionID          string     `json:"version_id"`
	Label              string     `json:"label"`
	Capacity           int        `json:"capacity"`
	Members            int        `json:"members"`
	StartsAt           *time.Time `json:"starts_at,omitempty"`
	EnrollmentClosesAt *time.Time `json:"enrollment_closes_at,omitempty"`
}
