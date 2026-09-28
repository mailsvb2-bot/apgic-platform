package specialist

import (
	"errors"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/marketplace"
)

var (
	ErrProfileNotFound    = errors.New("specialist profile not found")
	ErrCapabilityNotFound = errors.New("specialist capability not found")
	ErrProfileInvalid     = errors.New("specialist profile fields are invalid")
	ErrCapabilityInvalid  = errors.New("specialist capability is invalid")
	ErrEvidenceInvalid    = errors.New("specialist evidence is invalid")
)

const (
	VerificationNotApplicable = "NOT_APPLICABLE"
	VerificationPending       = "PENDING"
	VerificationActive        = "ACTIVE"

	EvidenceSubmitted = "SUBMITTED"
	EvidenceAccepted  = "ACCEPTED"
	EvidenceRejected  = "REJECTED"
)

type Capability struct {
	TopicID           string                    `json:"topic_id"`
	EvidenceState     marketplace.EvidenceState `json:"evidence_state"`
	VerificationState string                    `json:"verification_state"`
	EvidenceRefs      []string                  `json:"evidence_refs"`
}

type Evidence struct {
	ID         string     `json:"id"`
	TopicID    string     `json:"topic_id"`
	Kind       string     `json:"kind"`
	Reference  string     `json:"reference"`
	State      string     `json:"state"`
	SubmittedAt time.Time  `json:"submitted_at"`
	ReviewedAt *time.Time `json:"reviewed_at,omitempty"`
}

type Profile struct {
	ID              string                  `json:"id"`
	IdentityID      string                  `json:"identity_id"`
	DisplayName     string                  `json:"display_name"`
	ProfessionCode  string                  `json:"profession_code"`
	ProfileComplete bool                    `json:"profile_complete"`
	ReviewState     marketplace.ReviewState `json:"review_state"`
	Capabilities    []Capability            `json:"capabilities"`
	Evidence        []Evidence              `json:"evidence"`
	PublishedTopics []string                `json:"published_topics"`
}

type PublishResult struct {
	Allowed         bool     `json:"allowed"`
	ReasonCodes     []string `json:"reason_codes"`
	PublishedTopics []string `json:"published_topics"`
	PolicyVersion   string   `json:"policy_version"`
}

type Store interface {
	UpsertProfile(identityID, displayName, professionCode string) (Profile, error)
	Profile(identityID string) (Profile, error)
	DeclareCapability(identityID, topicID string) (Profile, error)
	SubmitEvidence(identityID, topicID, kind, reference string) (Profile, error)
	Publish(identityID, topicID string) (PublishResult, error)
	Unpublish(identityID, topicID string) (PublishResult, error)
}

func NormalizeProfile(displayName, professionCode string) (string, string, error) {
	displayName = strings.TrimSpace(displayName)
	professionCode = strings.ToUpper(strings.TrimSpace(professionCode))
	if displayName == "" || professionCode == "" {
		return "", "", ErrProfileInvalid
	}
	return displayName, professionCode, nil
}

func NormalizeTopic(topicID string) (string, error) {
	topicID = strings.ToLower(strings.TrimSpace(topicID))
	switch topicID {
	case "anxiety", "sleep", "relationships", "career":
		return topicID, nil
	default:
		return "", ErrCapabilityInvalid
	}
}

func NormalizeEvidence(kind, reference string) (string, string, error) {
	kind = strings.ToUpper(strings.TrimSpace(kind))
	reference = strings.TrimSpace(reference)
	if kind == "" || reference == "" {
		return "", "", ErrEvidenceInvalid
	}
	return kind, reference, nil
}
