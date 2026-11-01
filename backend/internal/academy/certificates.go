package academy

import "time"

// LearnerCertificate is the signed learner's durable, privacy-scoped credential
// readback. A historical certificate is distinct from current lesson access.
type LearnerCertificate struct {
	ID                     string    `json:"id"`
	EnrollmentID           string    `json:"enrollment_id"`
	CourseTitle            string    `json:"course_title"`
	CourseVersion          int       `json:"course_version"`
	State                  string    `json:"state"`
	LearningAccessState    string    `json:"learning_access_state"`
	VerificationURL        string    `json:"verification_url"`
	EvidenceCandidateState string    `json:"evidence_candidate_state"`
	IssuedAt               time.Time `json:"issued_at"`
}
