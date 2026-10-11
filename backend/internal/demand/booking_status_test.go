package demand

import (
	"errors"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"
)

func TestOwnerBookingResumeUsesCanonicalStateAndCannotLeakForeignBooking(t *testing.T) {
	store := &fakeJourneyStore{}
	service, err := NewConformanceServiceWithStores(nil, nil, store)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := service.CreateIntent("нужна помощь со сном")
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.CreateIntent("тревога")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmIntent(owner.ID, []string{"sleep"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	slots, err := service.Slots("spec-lebedeva")
	if err != nil || len(slots) == 0 {
		t.Fatalf("slots=%#v err=%v", slots, err)
	}
	hold, err := service.AcquireHold(owner.ID, slots[0].ID, owner.ClientIdentityID)
	if err != nil {
		t.Fatal(err)
	}

	// The hold is canonical before any Booking/Order row exists.
	before, err := service.ReadBookingStatus(hold.BookingID, owner.ClientIdentityID)
	if err != nil || before.State != booking.StateHeld || before.HoldID != hold.ID || before.StartsAt != nil {
		t.Fatalf("pre-checkout status=%#v err=%v", before, err)
	}
	if _, err := service.ReadBookingStatus(hold.BookingID, other.ClientIdentityID); !errors.Is(err, ErrBookingNotFound) {
		t.Fatalf("foreign booking must not disclose existence: %v", err)
	}
	if _, err := service.ReadBookingStatus("nonexistent-booking", owner.ClientIdentityID); !errors.Is(err, ErrBookingNotFound) {
		t.Fatalf("nonexistent booking: %v", err)
	}

	if _, err := service.CreateCheckout(hold.ID, owner.ClientIdentityID, "SBP"); err != nil {
		t.Fatal(err)
	}
	pending, err := service.ReadBookingStatus(hold.BookingID, owner.ClientIdentityID)
	if err != nil || pending.State != booking.StatePendingPayment || pending.StartsAt == nil || pending.EndsAt == nil {
		t.Fatalf("pending canonical booking=%#v err=%v", pending, err)
	}

	// Another API process commits payment confirmation and subsequent cancellation.
	for _, state := range []booking.State{booking.StateConfirmed, booking.StateCancelled} {
		for _, item := range store.snapshot.Bookings {
			if item != nil && item.ID == hold.BookingID {
				item.State = state
			}
		}
		reopened, readErr := NewConformanceServiceWithStores(nil, nil, store)
		if readErr != nil {
			t.Fatal(readErr)
		}
		actual, readErr := reopened.ReadBookingStatus(hold.BookingID, owner.ClientIdentityID)
		if readErr != nil || actual.State != state {
			t.Fatalf("restart status=%#v err=%v want=%s", actual, readErr, state)
		}
		// The original in-memory service must also refresh after a second process changed the booking.
		refreshed, refreshErr := service.ReadBookingStatus(hold.BookingID, owner.ClientIdentityID)
		if refreshErr != nil || refreshed.State != state {
			t.Fatalf("original process is stale: status=%#v err=%v want=%s", refreshed, refreshErr, state)
		}
	}
}
