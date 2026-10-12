package demand

import (
	"errors"
	"testing"
)

func TestOrderRecoveredByBookingAfterRestartWithoutRememberedOrderID(t *testing.T) {
	store := &fakeJourneyStore{}
	service, err := NewConformanceServiceWithStores(nil, nil, store)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := service.CreateIntent("бессонница")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmIntent(intent.ID, []string{"sleep"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	slots, err := service.Slots("spec-lebedeva")
	if err != nil || len(slots) == 0 {
		t.Fatalf("slots=%v err=%v", slots, err)
	}
	hold, err := service.AcquireHold(intent.ID, slots[0].ID, intent.ClientIdentityID)
	if err != nil {
		t.Fatal(err)
	}
	if state, err := service.ReadOrderForBooking(hold.BookingID, intent.ClientIdentityID); !errors.Is(err, ErrOrderNotFound) || state != nil {
		t.Fatalf("unpaid hold became order: %#v %v", state, err)
	}
	instruction, err := service.CreateCheckout(hold.ID, intent.ClientIdentityID, "SBP")
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := NewConformanceServiceWithStores(nil, nil, store)
	if err != nil {
		t.Fatal(err)
	}
	state, err := restarted.ReadOrderForBooking(hold.BookingID, intent.ClientIdentityID)
	if err != nil || state == nil || state.OrderID != instruction.OrderID ||
		state.BookingID != hold.BookingID || state.PaymentState != "PENDING_PROVIDER" {
		t.Fatalf("reopened payment status=%#v err=%v", state, err)
	}
	for _, tc := range []struct{ bookingID, identityID string }{
		{hold.BookingID, "foreign-client"},
		{"nonexistent-booking", intent.ClientIdentityID},
	} {
		if state, err := restarted.ReadOrderForBooking(tc.bookingID, tc.identityID); !errors.Is(err, ErrOrderNotFound) || state != nil {
			t.Fatalf("unowned order leaked: %#v %v", state, err)
		}
	}
}
