package demand

import (
	"errors"
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/communication"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/consultation"
)

const (
	fallbackProviderID = "comm-external-fallback"
	recoveryPolicy     = "communication-recovery-v1"
	recoveryNotice     = "Сбой связи у внешнего провайдера. APGIC не владеет комнатой, не завершает консультацию и повторно не списывает деньги."
	recoveredNotice    = "Связь восстановлена у внешнего провайдера. Консультация снова идёт. Повторной оплаты нет."
	failureNotice      = "Восстановление невозможно. Зафиксирован технический сбой. Возврат открыт у внешнего провайдера: APGIC деньги не принимает и не возвращает."
)

func (s *Service) ReportProviderFailure(bookingID, kind, evidenceRef string, recoverable bool) (*ConsultationView, error) {
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
		return nil, ErrConsultNotReady
	}
	if session.State == consultation.StateTechnicalFailure {
		view := sessionView(session, evidenceRef, true)
		view.RecoveryAction = string(communication.RecoveryRefund)
		view.ReasonCode = "COMM_RECOVERY_EXHAUSTED_REFUND"
		view.RefundPathOpened = s.reversals[s.orderIDForBookingLocked(bookingID)] != nil
		view.Notice = failureNotice
		return view, nil
	}
	attempt := 0
	if !recoverable {
		attempt = 1
	}
	decision, err := s.decideLocked(session, kind, evidenceRef, attempt)
	if err != nil {
		return nil, err
	}
	if decision.RefundPathRequired {
		return s.exhaustLocked(session, kind, evidenceRef, decision)
	}
	if session.State == consultation.StateRecovering {
		view := sessionView(session, evidenceRef, true)
		view.RecoveryAction = string(decision.Action)
		view.ReasonCode = decision.ReasonCode
		view.Notice = recoveryNotice
		return view, nil
	}
	if session.State != consultation.StateInProgress {
		return nil, ErrConsultNotReady
	}
	now := s.now().UTC()
	fact := consultation.Fact{
		ID: newID("fact-"), IdempotencyKey: "recovery-started:" + bookingID + ":" + itoa(recoveryCycles(session)+1),
		Type: consultation.FactRecoveryStarted, Role: consultation.RoleSystem,
		ProviderInstanceID: session.ProviderInstanceID,
		ProviderReference:  "provider-failure:" + bookingID,
		EvidenceRef:        evidenceRef, OccurredAt: now,
	}
	if err := session.RecordFact(fact); err != nil && !errors.Is(err, consultation.ErrDuplicateFact) {
		return nil, err
	}
	if decision.ProviderInstanceID != "" {
		session.ProviderInstanceID = decision.ProviderInstanceID
	}
	view := sessionView(session, evidenceRef, false)
	view.RecoveryAction = string(decision.Action)
	view.ReasonCode = decision.ReasonCode
	view.Notice = recoveryNotice
	return view, nil
}

func (s *Service) SucceedRecovery(bookingID, evidenceRef string) (*ConsultationView, error) {
	if strings.TrimSpace(evidenceRef) == "" {
		return nil, ErrConsultEvidence
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.sessionLocked(bookingID)
	if err != nil {
		return nil, err
	}
	if session.State == consultation.StateInProgress && s.hasRecoverySucceeded(session) {
		view := sessionView(session, evidenceRef, true)
		view.RecoveryAction = string(communication.RecoveryRetrySameProvider)
		view.ReasonCode = "COMM_RECOVERY_SUCCEEDED"
		view.Notice = recoveredNotice
		return view, nil
	}
	if session.State != consultation.StateRecovering {
		return nil, ErrConsultNotReady
	}
	now := s.now().UTC()
	fact := consultation.Fact{
		ID: newID("fact-"), IdempotencyKey: "recovery-succeeded:" + bookingID + ":" + itoa(recoveryCycles(session)),
		Type: consultation.FactRecoverySucceeded, Role: consultation.RoleSystem,
		ProviderInstanceID: session.ProviderInstanceID,
		ProviderReference:  "provider-recovery:" + bookingID,
		EvidenceRef:        evidenceRef, OccurredAt: now,
	}
	if err := session.RecordFact(fact); err != nil {
		if errors.Is(err, consultation.ErrDuplicateFact) {
			view := sessionView(session, evidenceRef, true)
			view.Notice = recoveredNotice
			return view, nil
		}
		return nil, err
	}
	view := sessionView(session, evidenceRef, false)
	view.RecoveryAction = string(communication.RecoveryRetrySameProvider)
	view.ReasonCode = "COMM_RECOVERY_SUCCEEDED"
	view.Notice = recoveredNotice
	return view, nil
}

func (s *Service) decideLocked(session *consultation.Session, kind, evidenceRef string, attempt int) (communication.RecoveryDecision, error) {
	decision, err := communication.DecideRecovery(communication.TechnicalFailure{
		ConsultationID:     session.ID,
		BookingID:          session.BookingID,
		ProviderInstanceID: session.ProviderInstanceID,
		Kind:               communication.FailureKind(kind),
		Attempt:            attempt,
		EvidenceRef:        evidenceRef,
		OccurredAt:         s.now().UTC(),
	}, communication.RecoveryPolicy{
		Version:             recoveryPolicy,
		MaxRecoveryAttempts: 1,
		AllowFallback:       true,
		ExhaustedAction:     communication.RecoveryRefund,
	}, communication.RecoveryContext{
		CurrentProviderID:   session.ProviderInstanceID,
		FallbackProviderID:  fallbackProviderID,
		RefundEligible:      true,
		RescheduleAvailable: true,
	})
	if errors.Is(err, communication.ErrInvalidRecoveryInput) {
		return communication.RecoveryDecision{}, ErrRecoveryInvalid
	}
	return decision, err
}

func (s *Service) exhaustLocked(session *consultation.Session, kind, evidenceRef string, decision communication.RecoveryDecision) (*ConsultationView, error) {
	if session.State != consultation.StateInProgress && session.State != consultation.StateRecovering {
		return nil, ErrConsultNotReady
	}
	now := s.now().UTC()
	fact := consultation.Fact{
		ID: newID("fact-"), IdempotencyKey: "technical-failure:" + session.BookingID,
		Type: consultation.FactTechnicalFailure, Role: consultation.RoleSystem,
		ProviderInstanceID: session.ProviderInstanceID,
		ProviderReference:  "provider-failure:" + kind + ":" + session.BookingID,
		EvidenceRef:        evidenceRef, OccurredAt: now,
	}
	if err := session.RecordFact(fact); err != nil && !errors.Is(err, consultation.ErrDuplicateFact) {
		return nil, err
	}
	if session.State != consultation.StateTechnicalFailure {
		return nil, ErrConsultNotReady
	}
	orderID := s.orderIDForBookingLocked(session.BookingID)
	if orderID == "" {
		return nil, ErrOrderNotFound
	}
	cancellation, err := s.cancelOrderLocked(orderID, "TECHNICAL_FAILURE", failureNotice)
	if err != nil {
		return nil, err
	}
	view := sessionView(session, evidenceRef, cancellation.Idempotent)
	view.RecoveryAction = string(decision.Action)
	view.ReasonCode = decision.ReasonCode
	view.RefundPathOpened = true
	view.APGICReturnsFunds = cancellation.APGICReturnsFunds
	view.Notice = failureNotice
	if view.APGICReturnsFunds || cancellation.APGICAcceptsFunds || cancellation.ExecutionOwner != "EXTERNAL_PROVIDER" {
		return nil, ErrCustodyForbidden
	}
	return view, nil
}

func (s *Service) orderIDForBookingLocked(bookingID string) string {
	for _, instruction := range s.instructions {
		if instruction.BookingID == bookingID {
			return instruction.OrderID
		}
	}
	return ""
}

func (s *Service) hasRecoverySucceeded(session *consultation.Session) bool {
	return recoveryCycles(session) > 0 && countFacts(session, consultation.FactRecoverySucceeded) == recoveryCycles(session)
}

func recoveryCycles(session *consultation.Session) int {
	return countFacts(session, consultation.FactRecoveryStarted)
}

func countFacts(session *consultation.Session, kind consultation.FactType) int {
	count := 0
	for _, fact := range session.Facts() {
		if fact.Type == kind {
			count++
		}
	}
	return count
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := []byte{}
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}
