package demand

import "strings"

// ReadOrderForBooking is used when a browser or native deep-link knows only
// the booking ID. Ownership is revalidated by ReadOrderStatus; an order ID is
// never treated as a bearer secret. No checkout means no payment record.
func (s *Service) ReadOrderForBooking(bookingID, identityID string) (*OrderStatus, error) {
	if s == nil || strings.TrimSpace(bookingID) == "" || strings.TrimSpace(identityID) == "" {
		return nil, ErrOrderNotFound
	}
	s.mu.Lock()
	if err := s.expireHoldsLocked(); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	booked := s.bookings[bookingID]
	if booked == nil || booked.ID != bookingID || booked.ClientIdentityID != identityID {
		s.mu.Unlock()
		return nil, ErrOrderNotFound
	}
	var orderID string
	for _, instruction := range s.instructions {
		if instruction != nil && instruction.BookingID == bookingID {
			orderID = instruction.OrderID
			break
		}
	}
	s.mu.Unlock()
	if orderID == "" {
		return nil, ErrOrderNotFound
	}
	return s.ReadOrderStatus(orderID, identityID)
}
