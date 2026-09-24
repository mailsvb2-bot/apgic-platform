package marketplace

import (
	"errors"
	"sort"
)

type EvidenceState string

const (
	EvidenceSelfDeclared      EvidenceState = "SELF_DECLARED"
	EvidenceDocumentSupported EvidenceState = "DOCUMENT_SUPPORTED"
	EvidenceAPGICVerified     EvidenceState = "APGIC_VERIFIED"
)

type ReviewState string

const (
	ReviewPending  ReviewState = "PENDING"
	ReviewManual   ReviewState = "MANUAL_REVIEW"
	ReviewApproved ReviewState = "APPROVED"
	ReviewRejected ReviewState = "REJECTED"
)

type PublishState string

const (
	PublishDraft       PublishState = "DRAFT"
	PublishPublished   PublishState = "PUBLISHED"
	PublishUnpublished PublishState = "UNPUBLISHED"
)

var (
	ErrInvalidCapability = errors.New("capability topic and supported evidence state are required")
	ErrInvalidProfile    = errors.New("specialist id and identity id are required")
)

type Capability struct {
	Topic        string
	Evidence     EvidenceState
	EvidenceRefs []string
}

func NewCapability(topic string, evidence EvidenceState, evidenceRefs ...string) (Capability, error) {
	if topic == "" || evidenceRank(evidence) == 0 {
		return Capability{}, ErrInvalidCapability
	}
	return Capability{Topic: topic, Evidence: evidence, EvidenceRefs: cloneStrings(evidenceRefs)}, nil
}

func (c Capability) Verified() bool {
	return c.Evidence == EvidenceAPGICVerified
}

type SpecialistProfile struct {
	ID              string
	IdentityID      string
	DisplayName     string
	Profession      string
	Complete        bool
	Review          ReviewState
	Capabilities    []Capability
	PublishState    PublishState
	PublishedTopics []string
}

func NewSpecialistProfile(id, identityID, displayName string) (*SpecialistProfile, error) {
	if id == "" || identityID == "" {
		return nil, ErrInvalidProfile
	}
	return &SpecialistProfile{
		ID:           id,
		IdentityID:   identityID,
		DisplayName:  displayName,
		Review:       ReviewPending,
		PublishState: PublishDraft,
	}, nil
}

func (s *SpecialistProfile) AddCapability(capability Capability) {
	for i, existing := range s.Capabilities {
		if existing.Topic == capability.Topic {
			s.Capabilities[i] = capability
			return
		}
	}
	s.Capabilities = append(s.Capabilities, capability)
	sort.Slice(s.Capabilities, func(i, j int) bool { return s.Capabilities[i].Topic < s.Capabilities[j].Topic })
}

func (s *SpecialistProfile) Capability(topic string) (Capability, bool) {
	for _, capability := range s.Capabilities {
		if capability.Topic == topic {
			return capability, true
		}
	}
	return Capability{}, false
}

func (s *SpecialistProfile) IsPublishedFor(topic string) bool {
	if s.PublishState != PublishPublished {
		return false
	}
	for _, publishedTopic := range s.PublishedTopics {
		if publishedTopic == topic {
			return true
		}
	}
	return false
}

func evidenceRank(state EvidenceState) int {
	switch state {
	case EvidenceSelfDeclared:
		return 1
	case EvidenceDocumentSupported:
		return 2
	case EvidenceAPGICVerified:
		return 3
	default:
		return 0
	}
}

func cloneStrings(values []string) []string {
	return append([]string(nil), values...)
}
