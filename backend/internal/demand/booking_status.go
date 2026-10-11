package demand

import (
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"
)

// BookingStatus is an owner-only projection of canonical booking truth.
// It intentionally contains no provider credentials, payment secrets or
// consultation content. The browser must not be a booking state owner.
type BookingStatus struct {
	BookingID     string        `json:"booking_id"`
	State         booking.State `json:"state"`
	HoldID        string        `json:"hold_id"`
	HoldExpiresAt time.Time     `json:"hold_expires_at"`
	StartsAt      *time.Time    `json:"starts_at,omitempty"`
	EndsAt        *time.Time    `json:"ends_at,omitempty"`
}

// ReadBookingStatus supports a new browser/device and a restarted API.
// It always refreshes canonical PostgreSQL journey state before the read.
// Looking up somebody else's UUID must not reveal whether it exists.
func (s *Service) ReadBookingStatus(bookingID, identityID string) (*BookingStatus, error) {
	if s == nil || strings.TrimSpace(bookingID) == "" || strings.TrimSpace(identityID) == "" {
		return nil, ErrBookingNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.expireHoldsLocked(); err != nil {
		return nil, err
	}
	if booked := s.bookings[bookingID]; booked != nil {
		if booked.ClientIdentityID != identityID {
			return nil, ErrBookingNotFound
		}
		start, end := booked.StartsAt, booked.EndsAt
		return &BookingStatus{
			BookingID: booked.ID, State: booked.State, HoldID: booked.HoldID,
			HoldExpiresAt: booked.HoldExpiresAt, StartsAt: &start, EndsAt: &end,
		}, nil
	}
	// Before checkout the canonical hold already has a booking ID, but a
	// Booking row need not exist yet. Do not invent confirmation/payment.
	for _, hold := range s.holds {
		if hold == nil || hold.BookingID != bookingID {
			continue
		}
		if hold.ClientIdentityID != identityID {
			return nil, ErrBookingNotFound
		}
		state := hold.BookingState
		if hold.State == "EXPIRED" {
			state = booking.StateExpired
		} else if state == "" {
			state = booking.StateHeld
		}
		return &BookingStatus{
			BookingID: bookingID, State: state, HoldID: hold.ID,
			HoldExpiresAt: hold.ExpiresAt,
		}, nil
	}
	return nil, ErrBookingNotFound
}
