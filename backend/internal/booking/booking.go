package booking

import (
	"errors"
	"strings"
	"time"
)

type State string

const (
	StateHeld           State = "HELD"
	StatePendingPayment State = "PENDING_PAYMENT"
	StateConfirmed      State = "CONFIRMED"
	StateCancelled      State = "CANCELLED"
	StateExpired        State = "EXPIRED"
	StateCompleted      State = "COMPLETED"
	StateNoShow         State = "NO_SHOW"
)

const (
	ReasonTransitionAllowed = "BOOK_TRANSITION_ALLOWED"
	ReasonTransitionDenied  = "BOOK_TRANSITION_DENIED"
	ReasonTooEarly          = "BOOK_TRANSITION_TOO_EARLY"
	ReasonHoldExpired       = "BOOK_HOLD_EXPIRED"
)

var (
	ErrInvalidBooking     = errors.New("invalid booking")
	ErrTransitionDenied   = errors.New("booking transition denied")
	ErrTransitionTooEarly = errors.New("booking transition is too early")
	ErrHoldExpired        = errors.New("booking hold expired")
)

type Booking struct {
	ID               string
	SlotID           string
	HoldID           string
	ClientIdentityID string
	State            State
	HoldExpiresAt    time.Time
	StartsAt         time.Time
	EndsAt           time.Time
	UpdatedAt        time.Time
}

type TransitionResult struct {
	From       State
	To         State
	ReasonCode string
}

func New(
	id,
	slotID,
	holdID,
	clientIdentityID string,
	holdExpiresAt,
	startsAt,
	endsAt,
	now time.Time,
) (*Booking, error) {
	if strings.TrimSpace(id) == "" ||
		strings.TrimSpace(slotID) == "" ||
		strings.TrimSpace(holdID) == "" ||
		strings.TrimSpace(clientIdentityID) == "" ||
		now.IsZero() ||
		holdExpiresAt.IsZero() ||
		startsAt.IsZero() ||
		endsAt.IsZero() ||
		!holdExpiresAt.After(now) ||
		!endsAt.After(startsAt) {
		return nil, ErrInvalidBooking
	}
	return &Booking{
		ID:               id,
		SlotID:           slotID,
		HoldID:           holdID,
		ClientIdentityID: clientIdentityID,
		State:            StateHeld,
		HoldExpiresAt:    holdExpiresAt,
		StartsAt:         startsAt,
		EndsAt:           endsAt,
		UpdatedAt:        now,
	}, nil
}

func (b *Booking) Transition(to State, now time.Time) (TransitionResult, error) {
	result := TransitionResult{From: b.State, To: to}
	if now.IsZero() {
		result.ReasonCode = ReasonTransitionDenied
		return result, ErrTransitionDenied
	}
	if b.State == StateHeld || b.State == StatePendingPayment {
		if !now.Before(b.HoldExpiresAt) && to != StateExpired {
			result.ReasonCode = ReasonHoldExpired
			return result, ErrHoldExpired
		}
	}

	if to == StateExpired {
		if (b.State != StateHeld && b.State != StatePendingPayment) || now.Before(b.HoldExpiresAt) {
			result.ReasonCode = ReasonTooEarly
			return result, ErrTransitionTooEarly
		}
	}
	if to == StateNoShow {
		if b.State != StateConfirmed || now.Before(b.StartsAt) {
			result.ReasonCode = ReasonTooEarly
			return result, ErrTransitionTooEarly
		}
	}
	if to == StateCompleted {
		if b.State != StateConfirmed || now.Before(b.EndsAt) {
			result.ReasonCode = ReasonTooEarly
			return result, ErrTransitionTooEarly
		}
	}
	if !allowedTransition(b.State, to) {
		result.ReasonCode = ReasonTransitionDenied
		return result, ErrTransitionDenied
	}

	b.State = to
	b.UpdatedAt = now
	result.ReasonCode = ReasonTransitionAllowed
	return result, nil
}

func allowedTransition(from, to State) bool {
	switch from {
	case StateHeld:
		return to == StatePendingPayment ||
			to == StateConfirmed ||
			to == StateCancelled ||
			to == StateExpired
	case StatePendingPayment:
		return to == StateConfirmed ||
			to == StateCancelled ||
			to == StateExpired
	case StateConfirmed:
		return to == StateCancelled ||
			to == StateCompleted ||
			to == StateNoShow
	default:
		return false
	}
}
