package consultation

import (
	"errors"
	"strings"
	"time"
)

type State string

const (
	StateScheduled        State = "SCHEDULED"
	StateReady            State = "READY"
	StateInProgress       State = "IN_PROGRESS"
	StateRecovering       State = "RECOVERING"
	StateTechnicalFailure State = "TECHNICAL_FAILURE"
	StateCompleted        State = "COMPLETED"
)

type FactType string

const (
	FactReady             FactType = "READY"
	FactJoined            FactType = "JOINED"
	FactLeft              FactType = "LEFT"
	FactStarted           FactType = "STARTED"
	FactRecoveryStarted   FactType = "RECOVERY_STARTED"
	FactRecoverySucceeded FactType = "RECOVERY_SUCCEEDED"
	FactTechnicalFailure  FactType = "TECHNICAL_FAILURE"
	FactEnded             FactType = "ENDED"
)

type ParticipantRole string

const (
	RoleClient     ParticipantRole = "CLIENT"
	RoleSpecialist ParticipantRole = "SPECIALIST"
	RoleSystem     ParticipantRole = "SYSTEM"
)

var (
	ErrInvalidSession      = errors.New("invalid consultation session")
	ErrInvalidFact         = errors.New("invalid consultation lifecycle fact")
	ErrDuplicateFact       = errors.New("duplicate consultation lifecycle fact")
	ErrTransitionDenied    = errors.New("consultation transition denied")
	ErrCompletionEvidence  = errors.New("consultation completion evidence required")
)

type Fact struct {
	ID                string
	IdempotencyKey    string
	Type              FactType
	Role              ParticipantRole
	IdentityID        string
	ProviderReference string
	EvidenceRef       string
	OccurredAt        time.Time
}

type CompletionEvidence struct {
	ID                string
	IdempotencyKey    string
	ProviderReference string
	EvidenceRef       string
	ObservedAt        time.Time
}

type Session struct {
	ID                   string
	BookingID            string
	ClientIdentityID     string
	SpecialistIdentityID string
	ProviderInstanceID   string
	State                State
	CreatedAt            time.Time
	UpdatedAt            time.Time
	facts                []Fact
	idempotencyKeys      map[string]struct{}
}

func New(
	id,
	bookingID,
	clientIdentityID,
	specialistIdentityID,
	providerInstanceID string,
	now time.Time,
) (*Session, error) {
	if strings.TrimSpace(id) == "" ||
		strings.TrimSpace(bookingID) == "" ||
		strings.TrimSpace(clientIdentityID) == "" ||
		strings.TrimSpace(specialistIdentityID) == "" ||
		clientIdentityID == specialistIdentityID ||
		strings.TrimSpace(providerInstanceID) == "" ||
		now.IsZero() {
		return nil, ErrInvalidSession
	}
	return &Session{
		ID: id, BookingID: bookingID,
		ClientIdentityID: clientIdentityID,
		SpecialistIdentityID: specialistIdentityID,
		ProviderInstanceID: providerInstanceID,
		State: StateScheduled, CreatedAt: now, UpdatedAt: now,
		idempotencyKeys: map[string]struct{}{},
	}, nil
}

func (s *Session) RecordFact(fact Fact) error {
	if err := s.validateFact(fact); err != nil {
		return err
	}
	if _, exists := s.idempotencyKeys[fact.IdempotencyKey]; exists {
		return ErrDuplicateFact
	}
	if !s.factAllowed(fact) {
		return ErrTransitionDenied
	}

	s.facts = append(s.facts, fact)
	s.idempotencyKeys[fact.IdempotencyKey] = struct{}{}
	s.UpdatedAt = fact.OccurredAt
	s.deriveState(fact)
	return nil
}

func (s *Session) Complete(evidence CompletionEvidence) error {
	if strings.TrimSpace(evidence.ID) == "" ||
		strings.TrimSpace(evidence.IdempotencyKey) == "" ||
		strings.TrimSpace(evidence.ProviderReference) == "" ||
		strings.TrimSpace(evidence.EvidenceRef) == "" ||
		evidence.ObservedAt.IsZero() {
		return ErrCompletionEvidence
	}
	if _, exists := s.idempotencyKeys[evidence.IdempotencyKey]; exists {
		return ErrDuplicateFact
	}
	if s.State != StateInProgress && s.State != StateRecovering {
		return ErrTransitionDenied
	}
	if !s.hasFact(FactStarted, "") {
		return ErrCompletionEvidence
	}
	if !s.hasParticipantJoin(RoleClient) || !s.hasParticipantJoin(RoleSpecialist) {
		return ErrCompletionEvidence
	}

	fact := Fact{
		ID: evidence.ID, IdempotencyKey: evidence.IdempotencyKey,
		Type: FactEnded, Role: RoleSystem,
		ProviderReference: evidence.ProviderReference,
		EvidenceRef: evidence.EvidenceRef,
		OccurredAt: evidence.ObservedAt,
	}
	if err := s.RecordFact(fact); err != nil {
		return err
	}
	s.State = StateCompleted
	return nil
}

func (s *Session) Facts() []Fact {
	out := make([]Fact, len(s.facts))
	copy(out, s.facts)
	return out
}

func (s *Session) validateFact(fact Fact) error {
	if strings.TrimSpace(fact.ID) == "" ||
		strings.TrimSpace(fact.IdempotencyKey) == "" ||
		fact.OccurredAt.IsZero() ||
		fact.OccurredAt.Before(s.CreatedAt) ||
		fact.OccurredAt.Before(s.UpdatedAt) {
		return ErrInvalidFact
	}
	switch fact.Type {
	case FactReady, FactJoined, FactLeft:
		if fact.Role != RoleClient && fact.Role != RoleSpecialist {
			return ErrInvalidFact
		}
		expected := s.ClientIdentityID
		if fact.Role == RoleSpecialist {
			expected = s.SpecialistIdentityID
		}
		if fact.IdentityID != expected ||
			strings.TrimSpace(fact.ProviderReference) == "" ||
			strings.TrimSpace(fact.EvidenceRef) == "" {
			return ErrInvalidFact
		}
	case FactStarted, FactRecoveryStarted, FactRecoverySucceeded, FactTechnicalFailure, FactEnded:
		if fact.Role != RoleSystem ||
			strings.TrimSpace(fact.ProviderReference) == "" ||
			strings.TrimSpace(fact.EvidenceRef) == "" {
			return ErrInvalidFact
		}
	default:
		return ErrInvalidFact
	}
	return nil
}

func (s *Session) factAllowed(fact Fact) bool {
	switch fact.Type {
	case FactReady:
		return s.State == StateScheduled || s.State == StateReady
	case FactJoined:
		return s.State == StateScheduled || s.State == StateReady
	case FactStarted:
		return (s.State == StateScheduled || s.State == StateReady) &&
			s.hasParticipantJoin(RoleClient) &&
			s.hasParticipantJoin(RoleSpecialist)
	case FactLeft:
		return s.State == StateInProgress || s.State == StateRecovering
	case FactRecoveryStarted:
		return s.State == StateInProgress
	case FactRecoverySucceeded:
		return s.State == StateRecovering
	case FactTechnicalFailure:
		return s.State == StateInProgress || s.State == StateRecovering
	case FactEnded:
		return s.State == StateInProgress || s.State == StateRecovering
	default:
		return false
	}
}

func (s *Session) deriveState(fact Fact) {
	switch fact.Type {
	case FactReady, FactJoined:
		if s.hasParticipantReadyOrJoin(RoleClient) && s.hasParticipantReadyOrJoin(RoleSpecialist) {
			s.State = StateReady
		}
	case FactStarted:
		s.State = StateInProgress
	case FactRecoveryStarted:
		s.State = StateRecovering
	case FactRecoverySucceeded:
		s.State = StateInProgress
	case FactTechnicalFailure:
		s.State = StateTechnicalFailure
	case FactEnded:
		s.State = StateCompleted
	}
}

func (s *Session) hasParticipantJoin(role ParticipantRole) bool {
	return s.hasFact(FactJoined, role)
}

func (s *Session) hasParticipantReadyOrJoin(role ParticipantRole) bool {
	return s.hasFact(FactReady, role) || s.hasFact(FactJoined, role)
}

func (s *Session) hasFact(kind FactType, role ParticipantRole) bool {
	for _, fact := range s.facts {
		if fact.Type == kind && (role == "" || fact.Role == role) {
			return true
		}
	}
	return false
}
