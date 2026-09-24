package booking

import (
	"testing"
	"time"
)

func newBooking(t *testing.T) (*Booking, time.Time) {
	t.Helper()
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	booking, err := New(
		"booking-1",
		"slot-1",
		"hold-1",
		"identity-1",
		now.Add(10*time.Minute),
		now.Add(time.Hour),
		now.Add(2*time.Hour),
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	return booking, now
}

func TestBookingAllowsOnlyExplicitTransitions(t *testing.T) {
	booking, now := newBooking(t)

	if _, err := booking.Transition(StatePendingPayment, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := booking.Transition(StateConfirmed, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if booking.State != StateConfirmed {
		t.Fatalf("state = %s", booking.State)
	}

	before := *booking
	result, err := booking.Transition(StateExpired, now.Add(20*time.Minute))
	if err != ErrTransitionTooEarly || result.ReasonCode != ReasonTooEarly {
		t.Fatalf("confirmed -> expired result=%#v err=%v", result, err)
	}
	if *booking != before {
		t.Fatal("denied transition mutated booking")
	}
}

func TestExpiredHoldCannotConfirm(t *testing.T) {
	booking, now := newBooking(t)
	result, err := booking.Transition(StateConfirmed, now.Add(11*time.Minute))
	if err != ErrHoldExpired || result.ReasonCode != ReasonHoldExpired {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if booking.State != StateHeld {
		t.Fatalf("expired hold silently mutated state to %s", booking.State)
	}

	if _, err := booking.Transition(StateExpired, now.Add(11*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if booking.State != StateExpired {
		t.Fatalf("state = %s", booking.State)
	}
}

func TestNoShowAndCompletionRespectScheduledTime(t *testing.T) {
	booking, now := newBooking(t)
	if _, err := booking.Transition(StateConfirmed, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	before := *booking
	if _, err := booking.Transition(StateNoShow, now.Add(30*time.Minute)); err != ErrTransitionTooEarly {
		t.Fatalf("early no-show err=%v", err)
	}
	if *booking != before {
		t.Fatal("early no-show mutated booking")
	}

	if _, err := booking.Transition(StateNoShow, now.Add(70*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if booking.State != StateNoShow {
		t.Fatalf("state = %s", booking.State)
	}
}

func TestTerminalBookingCannotReopen(t *testing.T) {
	booking, now := newBooking(t)
	if _, err := booking.Transition(StateCancelled, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	before := *booking
	result, err := booking.Transition(StateConfirmed, now.Add(2*time.Minute))
	if err != ErrTransitionDenied || result.ReasonCode != ReasonTransitionDenied {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if *booking != before {
		t.Fatal("terminal booking reopened")
	}
}

func TestBookingRejectsHoldOutsideSlotTimeline(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	_, err := New(
		"booking-invalid",
		"slot-invalid",
		"hold-invalid",
		"identity-1",
		now.Add(90*time.Minute),
		now.Add(time.Hour),
		now.Add(2*time.Hour),
		now,
	)
	if err != ErrInvalidBooking {
		t.Fatalf("err=%v", err)
	}
}
