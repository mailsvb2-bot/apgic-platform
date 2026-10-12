package demand

import (
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/refunds"
)

// OrderStatus is an owner-scoped snapshot of three *independent* business
// truths. A confirmed booking is not a substitute for captured payment.
type OrderStatus struct {
	OrderID       string        `json:"order_id"`
	BookingID     string        `json:"booking_id"`
	BookingState  booking.State `json:"booking_state"`
	PaymentState  string        `json:"payment_state"`
	RefundState   refunds.State `json:"refund_state,omitempty"`
	AmountMinor   int64         `json:"amount_minor"`
	Currency      string        `json:"currency"`
	ProviderID    string        `json:"provider_id"`
	PaymentMethod string        `json:"payment_method"`
}

// ReadOrderStatus reads authoritative durable journey state (when configured)
// and intentionally does not reveal whether a foreign order exists.
func (s *Service) ReadOrderStatus(orderID, clientIdentityID string) (*OrderStatus, error) {
	if s == nil || strings.TrimSpace(orderID) == "" || strings.TrimSpace(clientIdentityID) == "" {
		return nil, ErrOrderNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.expireHoldsLocked(); err != nil { return nil, err }
	instruction := s.instructionByOrderLocked(orderID)
	if instruction == nil { return nil, ErrOrderNotFound }
	booked := s.bookings[instruction.BookingID]
	if booked == nil || booked.ID != instruction.BookingID || booked.ClientIdentityID != clientIdentityID {
		return nil, ErrOrderNotFound
	}
	status := &OrderStatus{
		OrderID: instruction.OrderID, BookingID: booked.ID, BookingState: booked.State,
		PaymentState: "PENDING_PROVIDER", AmountMinor: instruction.AmountMinor,
		Currency: instruction.Currency, ProviderID: instruction.ProviderID,
		PaymentMethod: instruction.MethodCode,
	}
	if key, ok := s.orderEvidence[orderID]; ok {
		evidence := s.evidence[key]
		if evidence == nil || evidence.OrderID != orderID || evidence.BookingID != booked.ID ||
			evidence.ProviderID != instruction.ProviderID || evidence.AmountMinor != instruction.AmountMinor ||
			evidence.Currency != instruction.Currency {
			return nil, ErrEvidenceMismatch
		}
		status.PaymentState = "CAPTURED"
	}
	if cancellation := s.reversals[orderID]; cancellation != nil {
		if status.PaymentState != "CAPTURED" ||
			cancellation.BookingID != booked.ID || cancellation.ProviderID != instruction.ProviderID ||
			cancellation.AmountMinor != instruction.AmountMinor || cancellation.Currency != instruction.Currency {
			return nil, ErrEvidenceMismatch
		}
		status.RefundState = cancellation.RefundState
	}
	return status, nil
}
