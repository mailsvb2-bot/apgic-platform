package demand

import (
	"errors"
	"fmt"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"
)

type JourneySnapshot struct {
	Intents  []*Intent
	Holds    []*Hold
	Bookings []*booking.Booking
}

var ErrJourneyStoreProtocol = errors.New("unknown journey store reason code")

type JourneyStore interface {
	BootstrapCatalog(slots []Slot) ([]Slot, error)
	LoadJourney(slots []Slot) (JourneySnapshot, error)
	CreateIntent(intent *Intent) error
	ConfirmIntent(intent *Intent, confirmedAt time.Time) error
	AcquireHold(hold *Hold, slot Slot, now time.Time) (reasonCode string, err error)
	CreateBooking(booked *booking.Booking, now time.Time) (reasonCode string, err error)
	Expire(now time.Time) error
	UpdateBooking(booked *booking.Booking) error
}

func journeyReasonError(reason string) error {
	switch reason {
	case "", "BOOK_HOLD_ACQUIRED", "BOOK_CREATED":
		return nil
	case "BOOK_SLOT_NOT_FOUND":
		return ErrSlotNotFound
	case "BOOK_SLOT_NOT_EXCLUSIVE":
		return ErrSlotNotExclusive
	case "BOOK_SLOT_NOT_AVAILABLE":
		return ErrSlotUnavailable
	case "BOOK_SLOT_BOOKED":
		return ErrSlotBooked
	case "BOOK_SLOT_HELD":
		return ErrSlotHeld
	case "BOOK_HOLD_NOT_FOUND":
		return ErrHoldNotFound
	case "BOOK_HOLD_NOT_ACTIVE", "BOOK_HOLD_EXPIRED":
		return ErrHoldNotActive
	default:
		return fmt.Errorf("%w: %s", ErrJourneyStoreProtocol, reason)
	}
}
