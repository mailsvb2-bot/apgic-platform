package demand

import "github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"

// capturedPaymentForBookingLocked is used by all conformance consultation
// admission paths. Bookings must never themselves mint payment entitlement.
// Call with the service lock held after refreshJourneyLocked.
func (s *Service) capturedPaymentForBookingLocked(booked *booking.Booking) (string, bool) {
	if s == nil || booked == nil || booked.State != booking.StateConfirmed {
		return "", false
	}
	for _, instruction := range s.instructions {
		if instruction == nil || instruction.BookingID != booked.ID ||
			instruction.HoldID != booked.HoldID || instruction.OrderID == "" ||
			instruction.ProviderID == "" || instruction.AmountMinor <= 0 ||
			instruction.Currency == "" {
			continue
		}
		key, exists := s.orderEvidence[instruction.OrderID]
		if !exists || key == "" || s.reversals[instruction.OrderID] != nil {
			continue
		}
		evidence := s.evidence[key]
		if evidence == nil || evidence.OrderID != instruction.OrderID ||
			evidence.BookingID != booked.ID ||
			evidence.ProviderID != instruction.ProviderID ||
			evidence.ProviderEventID == "" || evidence.LedgerEntryID == "" ||
			evidence.AmountMinor != instruction.AmountMinor ||
			evidence.Currency != instruction.Currency ||
			key != evidence.ProviderID + "/" + evidence.ProviderEventID {
			continue
		}
		return key, true
	}
	return "", false
}
