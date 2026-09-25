package demand

import (
	"errors"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/consultation"
)

const (
	sessionProviderID = "comm-external"
	sessionNotice     = "Консультацию завершает доказательство внешнего провайдера связи. Таймер и присутствие в интерфейсе APGIC статус не ставят. Повторной оплаты нет."
)

type ConsultationView struct {
	ID                string `json:"id"`
	BookingID         string `json:"booking_id"`
	State             string `json:"state"`
	ProviderID        string `json:"provider_id"`
	EvidenceRef       string `json:"evidence_ref,omitempty"`
	APGICOwnsRoom     bool   `json:"apgic_owns_room"`
	ChargedAgain      bool   `json:"charged_again"`
	RefundPathOpened  bool   `json:"refund_path_opened"`
	APGICReturnsFunds bool   `json:"apgic_returns_funds"`
	RecoveryAction    string `json:"recovery_action,omitempty"`
	ReasonCode        string `json:"reason_code,omitempty"`
	Idempotent        bool   `json:"idempotent"`
	Notice            string `json:"notice"`
}

func (s *Service) RecordSessionPresence(bookingID string) (*ConsultationView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.sessionLocked(bookingID)
	if err != nil {
		return nil, err
	}
	if session.State == consultation.StateCompleted {
		return sessionView(session, "", true), nil
	}
	now := s.now().UTC()
	steps := []consultation.Fact{
		joinFact(session, consultation.RoleClient, session.ClientIdentityID, now),
		joinFact(session, consultation.RoleSpecialist, session.SpecialistIdentityID, now),
		{
			ID: newID("fact-"), IdempotencyKey: "started:" + bookingID, Type: consultation.FactStarted,
			Role: consultation.RoleSystem, ProviderInstanceID: sessionProviderID,
			ProviderReference: "provider-start:" + bookingID, EvidenceRef: "provider-fact:started:" + bookingID,
			OccurredAt: now,
		},
	}
	for _, fact := range steps {
		if err := session.RecordFact(fact); err != nil && !errors.Is(err, consultation.ErrDuplicateFact) {
			return nil, err
		}
	}
	if session.State != consultation.StateInProgress {
		return nil, ErrConsultNotReady
	}
	return sessionView(session, "", false), nil
}

func (s *Service) CompleteSession(bookingID, evidenceRef string) (*ConsultationView, error) {
	if strings.TrimSpace(evidenceRef) == "" {
		return nil, ErrConsultEvidence
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.sessionLocked(bookingID)
	if err != nil {
		return nil, err
	}
	if session.State == consultation.StateCompleted {
		return sessionView(session, evidenceRef, true), nil
	}
	if session.State != consultation.StateInProgress && session.State != consultation.StateRecovering {
		return nil, ErrConsultNotReady
	}
	now := s.now().UTC()
	evidence := consultation.CompletionEvidence{
		ID: newID("fact-"), IdempotencyKey: "ended:" + bookingID,
		ProviderInstanceID: sessionProviderID, ProviderReference: "provider-end:" + bookingID,
		EvidenceRef: evidenceRef, ObservedAt: now,
	}
	if err := session.Complete(evidence); err != nil {
		if errors.Is(err, consultation.ErrDuplicateFact) {
			return sessionView(session, evidenceRef, true), nil
		}
		if errors.Is(err, consultation.ErrCompletionEvidence) {
			return nil, ErrConsultEvidence
		}
		return nil, err
	}
	return sessionView(session, evidenceRef, false), nil
}

func (s *Service) sessionLocked(bookingID string) (*consultation.Session, error) {
	booked := s.bookings[bookingID]
	if booked == nil {
		return nil, ErrOrderNotFound
	}
	if booked.State != booking.StateConfirmed && s.sessions[bookingID] == nil {
		return nil, ErrConsultNotReady
	}
	if existing := s.sessions[bookingID]; existing != nil {
		return existing, nil
	}
	slot, ok := s.slot(booked.SlotID)
	if !ok {
		return nil, ErrSlotNotFound
	}
	session, err := consultation.New(
		newID("sess-"), bookingID, booked.ClientIdentityID, s.recipientLocked(slot.SpecialistID), sessionProviderID, s.now().UTC(),
	)
	if err != nil {
		return nil, err
	}
	s.sessions[bookingID] = session
	return session, nil
}

func joinFact(session *consultation.Session, role consultation.ParticipantRole, identityID string, now time.Time) consultation.Fact {
	return consultation.Fact{
		ID: newID("fact-"), IdempotencyKey: "joined:" + string(role) + ":" + session.BookingID,
		Type: consultation.FactJoined, Role: role, IdentityID: identityID,
		ProviderInstanceID: sessionProviderID,
		ProviderReference:  "provider-join:" + string(role) + ":" + session.BookingID,
		EvidenceRef:        "provider-fact:joined:" + string(role) + ":" + session.BookingID,
		OccurredAt:         now,
	}
}

func sessionView(session *consultation.Session, evidenceRef string, idempotent bool) *ConsultationView {
	return &ConsultationView{
		ID: session.ID, BookingID: session.BookingID, State: string(session.State),
		ProviderID: session.ProviderInstanceID, EvidenceRef: evidenceRef,
		APGICOwnsRoom: false, ChargedAgain: false, Idempotent: idempotent, Notice: sessionNotice,
	}
}
