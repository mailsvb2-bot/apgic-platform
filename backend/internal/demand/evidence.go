package demand

import (
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/ledger"
)

const evidenceNotice = "Внешний провайдер подтвердил списание в пользу специалиста. APGIC деньги не получил. Повтор того же сообщения не создаёт вторую запись."

type ProviderEvent struct {
	ProviderID      string
	ProviderEventID string
	OrderID         string
	AmountMinor     int64
	Currency        string
	Outcome         string
}

type PaymentEvidence struct {
	ID                string        `json:"id"`
	OrderID           string        `json:"order_id"`
	BookingID         string        `json:"booking_id"`
	BookingState      booking.State `json:"booking_state"`
	ProviderID        string        `json:"provider_id"`
	ProviderEventID   string        `json:"provider_event_id"`
	LedgerEntryID     string        `json:"ledger_entry_id"`
	AmountMinor       int64         `json:"amount_minor"`
	Currency          string        `json:"currency"`
	DebitAccountRef   string        `json:"debit_account_ref"`
	CreditAccountRef  string        `json:"credit_account_ref"`
	APGICAcceptsFunds bool          `json:"apgic_accepts_funds"`
	Idempotent        bool          `json:"idempotent"`
	Notice            string        `json:"notice"`
}

func (s *Service) ApplyProviderEvent(event ProviderEvent) (*PaymentEvidence, error) {
	if event.Outcome != "CAPTURED" || strings.TrimSpace(event.ProviderEventID) == "" {
		return nil, ErrEvidenceMismatch
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := event.ProviderID + "/" + event.ProviderEventID
	if existing := s.evidence[key]; existing != nil {
		copyEvidence := *existing
		copyEvidence.Idempotent = true
		return &copyEvidence, nil
	}
	instruction := s.instructionByOrderLocked(event.OrderID)
	if instruction == nil {
		return nil, ErrOrderNotFound
	}
	if instruction.ProviderID != event.ProviderID ||
		instruction.AmountMinor != event.AmountMinor ||
		instruction.Currency != strings.ToUpper(strings.TrimSpace(event.Currency)) {
		return nil, ErrEvidenceMismatch
	}
	if _, taken := s.orderEvidence[instruction.OrderID]; taken {
		return nil, ErrDuplicateEffect
	}
	debit := "external-provider/" + instruction.ProviderID + "/settlement"
	credit := instruction.PaymentRecipientID
	if err := rejectCustody(credit, instruction.ExecutionOwner); err != nil {
		return nil, err
	}
	if strings.Contains(strings.ToLower(debit), "apgic") {
		return nil, ErrCustodyForbidden
	}
	now := s.now().UTC()
	entry, err := ledger.NewEntry(ledger.Entry{
		ID:                  newID("led-"),
		DebitAccountRef:     debit,
		CreditAccountRef:    credit,
		AmountMinor:         instruction.AmountMinor,
		Currency:            instruction.Currency,
		ProviderEvidenceRef: key,
		EconomicEventRef:    instruction.OrderID,
		CorrelationID:       instruction.ID,
		OccurredAt:          now,
	})
	if err != nil {
		return nil, err
	}
	booked := s.bookings[instruction.BookingID]
	result, err := booked.Transition(booking.StateConfirmed, now)
	if err != nil {
		return nil, err
	}
	if hold := s.holds[instruction.HoldID]; hold != nil {
		hold.BookingState = result.To
	}
	instruction.BookingState = result.To
	created := &PaymentEvidence{
		ID:                newID("evi-"),
		OrderID:           instruction.OrderID,
		BookingID:         instruction.BookingID,
		BookingState:      result.To,
		ProviderID:        instruction.ProviderID,
		ProviderEventID:   event.ProviderEventID,
		LedgerEntryID:     entry.ID,
		AmountMinor:       entry.AmountMinor,
		Currency:          entry.Currency,
		DebitAccountRef:   entry.DebitAccountRef,
		CreditAccountRef:  entry.CreditAccountRef,
		APGICAcceptsFunds: false,
		Notice:            evidenceNotice,
	}
	s.evidence[key] = created
	s.orderEvidence[instruction.OrderID] = key
	s.recordBookingNoticeLocked(instruction.BookingID, now)
	copyEvidence := *created
	return &copyEvidence, nil
}

func (s *Service) instructionByOrderLocked(orderID string) *CheckoutInstruction {
	for _, instruction := range s.instructions {
		if instruction.OrderID == orderID {
			return instruction
		}
	}
	return nil
}
