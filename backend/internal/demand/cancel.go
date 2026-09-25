package demand

import (
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/ledger"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/refunds"
)

const cancelNotice = "Отмену исполняет тот же внешний провайдер. Он возвращает деньги плательщику. APGIC не принимает и не возвращает деньги. Исходная оплата сохраняется."

type Cancellation struct {
	ID                string        `json:"id"`
	OrderID           string        `json:"order_id"`
	BookingID         string        `json:"booking_id"`
	BookingState      booking.State `json:"booking_state"`
	RefundID          string        `json:"refund_id"`
	RefundState       refunds.State `json:"refund_state"`
	ProviderID        string        `json:"provider_id"`
	ExecutionOwner    string        `json:"execution_owner"`
	OriginalLedgerID  string        `json:"original_ledger_id"`
	ReversalLedgerID  string        `json:"reversal_ledger_id"`
	AmountMinor       int64         `json:"amount_minor"`
	Currency          string        `json:"currency"`
	APGICAcceptsFunds bool          `json:"apgic_accepts_funds"`
	APGICReturnsFunds bool          `json:"apgic_returns_funds"`
	Idempotent        bool          `json:"idempotent"`
	Notice            string        `json:"notice"`
}

func (s *Service) CancelOrder(orderID, reasonCode string) (*Cancellation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cancelOrderLocked(orderID, reasonCode, cancelNotice)
}

func (s *Service) cancelOrderLocked(orderID, reasonCode, notice string) (*Cancellation, error) {
	if strings.TrimSpace(reasonCode) == "" {
		reasonCode = "CLIENT_CANCEL"
	}
	if existing := s.reversals[orderID]; existing != nil {
		copyCancellation := *existing
		copyCancellation.Idempotent = true
		return &copyCancellation, nil
	}
	instruction := s.instructionByOrderLocked(orderID)
	evidenceKey, ok := s.orderEvidence[orderID]
	if instruction == nil || !ok {
		return nil, ErrOrderNotFound
	}
	evidence := s.evidence[evidenceKey]
	booked := s.bookings[instruction.BookingID]
	if evidence == nil || booked == nil || booked.State != booking.StateConfirmed {
		return nil, ErrCancelNotAllowed
	}
	now := s.now().UTC()
	refund, err := refunds.New(refunds.Request{
		ID:                       newID("ref-"),
		BookingID:                instruction.BookingID,
		OrderID:                  orderID,
		OriginalPaymentAttemptID: evidence.ID,
		OriginalProviderID:       evidence.ProviderID,
		ProviderID:               evidence.ProviderID,
		AmountMinor:              evidence.AmountMinor,
		Currency:                 evidence.Currency,
		PolicyVersion:            "cancellation-conformance-v1",
		PolicyDecision:           "ALLOW",
		ReasonCode:               reasonCode,
		IdempotencyKey:           "cancel:" + orderID,
		ExecutionOwner:           refunds.ExternalExecutionOwner,
		CreatedAt:                now,
	})
	if err != nil {
		return nil, err
	}
	providerRef := "provider-refund:" + orderID
	if err := refund.Transition(refunds.StateSent, now, providerRef, ""); err != nil {
		return nil, err
	}
	if err := refund.Transition(refunds.StateSucceeded, now, providerRef, "provider-evidence:"+orderID); err != nil {
		return nil, err
	}
	if err := refund.ValidateExecutionProvider(evidence.ProviderID); err != nil {
		return nil, err
	}
	reversal, err := ledger.NewEntry(ledger.Entry{
		ID:                  newID("led-"),
		DebitAccountRef:     evidence.CreditAccountRef,
		CreditAccountRef:    evidence.DebitAccountRef,
		AmountMinor:         evidence.AmountMinor,
		Currency:            evidence.Currency,
		ProviderEvidenceRef: refund.ProviderEvidenceRef,
		EconomicEventRef:    "reversal:" + orderID,
		CorrelationID:       refund.ID,
		OccurredAt:          now,
	})
	if err != nil {
		return nil, err
	}
	if strings.Contains(strings.ToLower(reversal.DebitAccountRef), "apgic") || strings.Contains(strings.ToLower(reversal.CreditAccountRef), "apgic") {
		return nil, ErrCustodyForbidden
	}
	if _, err := booked.Transition(booking.StateCancelled, now); err != nil {
		return nil, err
	}
	if hold := s.holds[instruction.HoldID]; hold != nil {
		hold.BookingState = booking.StateCancelled
	}
	instruction.BookingState = booking.StateCancelled
	created := &Cancellation{
		ID:                refund.ID,
		OrderID:           orderID,
		BookingID:         instruction.BookingID,
		BookingState:      booking.StateCancelled,
		RefundID:          refund.ID,
		RefundState:       refund.State,
		ProviderID:        evidence.ProviderID,
		ExecutionOwner:    refunds.ExternalExecutionOwner,
		OriginalLedgerID:  evidence.LedgerEntryID,
		ReversalLedgerID:  reversal.ID,
		AmountMinor:       evidence.AmountMinor,
		Currency:          evidence.Currency,
		APGICAcceptsFunds: false,
		APGICReturnsFunds: false,
		Notice:            notice,
	}
	if created.OriginalLedgerID == created.ReversalLedgerID || evidence.LedgerEntryID == "" {
		return nil, ErrCancelNotAllowed
	}
	s.reversals[orderID] = created
	copyCancellation := *created
	return &copyCancellation, nil
}
