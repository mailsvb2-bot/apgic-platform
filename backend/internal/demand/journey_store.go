package demand

import (
	"errors"
	"fmt"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/commerce"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/legal"
)

type JourneySnapshot struct {
	Intents      []*Intent
	Holds        []*Hold
	Bookings     []*booking.Booking
	Instructions []*CheckoutInstruction
	Evidence     []*PaymentEvidence
	Reversals    []*Cancellation
}

type CheckoutPersistence struct {
	Booking              *booking.Booking
	Instruction          *CheckoutInstruction
	LegalSnapshot        legal.TransactionSnapshot
	Order                commerce.OrderSnapshot
	RoutingPolicyVersion string
	DecidedAt            time.Time
}

var (
	ErrJourneyStoreProtocol   = errors.New("unknown journey store reason code")
	ErrCheckoutAlreadyExists = errors.New("checkout already exists")
)

type JourneyStore interface {
	BootstrapCatalog(slots []Slot) ([]Slot, error)
	LoadJourney(slots []Slot) (JourneySnapshot, error)
	CreateIntent(intent *Intent) error
	ConfirmIntent(intent *Intent, confirmedAt time.Time) error
	AcquireHold(hold *Hold, slot Slot, now time.Time) (reasonCode string, err error)
	CreateCheckout(persistence CheckoutPersistence) (reasonCode string, err error)
	Expire(now time.Time) error
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
	case "BOOK_CHECKOUT_ALREADY_EXISTS":
		return ErrCheckoutAlreadyExists
	default:
		return fmt.Errorf("%w: %s", ErrJourneyStoreProtocol, reason)
	}
}
