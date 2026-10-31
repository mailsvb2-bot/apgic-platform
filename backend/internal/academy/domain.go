// Package academy defines provider-neutral APGIC EdTech domain contracts.
// A completion/certificate is evidence of learning, never a SpecialistQualification.
package academy

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalid     = errors.New("academy invalid input")
	ErrNotFound    = errors.New("academy resource not found")
	ErrDenied      = errors.New("academy authorization denied")
	ErrEntitlement = errors.New("academy verified active entitlement required")
	ErrState       = errors.New("academy state transition invalid")
	ErrConflict    = errors.New("academy concurrent or duplicate mutation")
	ErrUnavailable = errors.New("academy storage unavailable")
)

type LessonKind string

const (
	Reading    LessonKind = "READING"
	Video      LessonKind = "VIDEO"
	Assessment LessonKind = "ASSESSMENT"
)

type Course struct {
	ID          string `json:"id"`
	ProductID   string `json:"product_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	AuthorID    string `json:"author_id,omitempty"`
	State       string `json:"state"`
	VersionID   string `json:"version_id,omitempty"`
	VersionNo   int    `json:"version_no,omitempty"`
}

// OriginalRightsDeclaration is only an authenticated author's declaration;
// it does not independently verify ownership or a third-party content license.
type OriginalRightsDeclaration struct {
	ID               string    `json:"id"`
	VersionID        string    `json:"version_id"`
	CurriculumSHA256 string    `json:"curriculum_sha256"`
	PolicyVersion    string    `json:"policy_version"`
	ProvenanceRef    string    `json:"provenance_ref"`
	AttestedAt       time.Time `json:"attested_at"`
}
type Module struct {
	ID        string `json:"id"`
	VersionID string `json:"version_id"`
	Ordinal   int    `json:"ordinal"`
	Title     string `json:"title"`
}
type Lesson struct {
	ID            string     `json:"id"`
	ModuleID      string     `json:"module_id"`
	Ordinal       int        `json:"ordinal"`
	Title         string     `json:"title"`
	Kind          LessonKind `json:"kind"`
	Body          string     `json:"body,omitempty"`
	MediaAssetRef string     `json:"media_asset_ref,omitempty"`
	PassingScore  int        `json:"passing_score,omitempty"`
	RubricVersion string     `json:"rubric_version,omitempty"`
	Completed     bool       `json:"completed,omitempty"`
}
type Enrollment struct {
	ID            string     `json:"id"`
	CourseID      string     `json:"course_id,omitempty"`
	CourseTitle   string     `json:"course_title,omitempty"`
	VersionNo     int        `json:"version_no,omitempty"`
	IdentityID    string     `json:"identity_id,omitempty"`
	VersionID     string     `json:"version_id"`
	EntitlementID string     `json:"entitlement_id,omitempty"`
	AccessState   string     `json:"access_state"`
	State         string     `json:"state"`
	EnrolledAt    time.Time  `json:"enrolled_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
}
type Attempt struct {
	ID            string     `json:"id"`
	EnrollmentID  string     `json:"enrollment_id"`
	LessonID      string     `json:"lesson_id"`
	AttemptNo     int        `json:"attempt_no"`
	Result        string     `json:"result"`
	Score         *int       `json:"score,omitempty"`
	RubricVersion string     `json:"rubric_version"`
	SubmittedAt   time.Time  `json:"submitted_at"`
	EvaluatedAt   *time.Time `json:"evaluated_at,omitempty"`
}
type Certificate struct {
	ID                     string    `json:"id"`
	EnrollmentID           string    `json:"enrollment_id"`
	VerificationToken      string    `json:"verification_token,omitempty"`
	VerificationURL        string    `json:"verification_url"`
	State                  string    `json:"state"`
	VersionID              string    `json:"version_id"`
	EvidenceCandidateState string    `json:"evidence_candidate_state"`
	IssuedAt               time.Time `json:"issued_at"`
}

func ValidateLesson(l Lesson) error {
	if l.Ordinal <= 0 || strings.TrimSpace(l.Title) == "" {
		return ErrInvalid
	}
	switch l.Kind {
	case Reading:
		if strings.TrimSpace(l.Body) == "" {
			return ErrInvalid
		}
	case Video:
		if strings.TrimSpace(l.MediaAssetRef) == "" || strings.Contains(l.MediaAssetRef, "://") {
			return ErrInvalid
		}
	case Assessment:
		if l.PassingScore < 1 || l.PassingScore > 100 || strings.TrimSpace(l.RubricVersion) == "" {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

func CanMarkProgress(kind LessonKind, passingAttempt bool, providerEvidenceVerified bool) error {
	switch kind {
	case Reading:
		return nil
	case Assessment:
		if passingAttempt {
			return nil
		}
	case Video:
		if providerEvidenceVerified {
			return nil
		}
	}
	return ErrDenied
}

func CanIssueCertificate(total, completed int, entitlementActive, enrollmentActive bool) error {
	if !entitlementActive {
		return ErrEntitlement
	}
	if !enrollmentActive || total == 0 || completed != total {
		return ErrState
	}
	return nil
}

// Learning result never bypasses the independent verification/qualification owner.
const EvidenceCandidateStatus = "PENDING_VERIFICATION"

type ReviewAttempt struct {
	AttemptID     string    `json:"attempt_id"`
	EnrollmentID  string    `json:"enrollment_id"`
	LessonID      string    `json:"lesson_id"`
	AttemptNo     int       `json:"attempt_no"`
	ResponseText  string    `json:"response_text"`
	SubmittedAt   time.Time `json:"submitted_at"`
	RubricVersion string    `json:"rubric_version"`
	PassingScore  int       `json:"passing_score"`
	CourseTitle   string    `json:"course_title"`
	LessonTitle   string    `json:"lesson_title"`
}
