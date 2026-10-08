package demand

import (
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"
)

func TestConfirmedBookingNotifiesWithoutMarketingOptInAndGatesJoin(t *testing.T) {
	service := NewConformanceService(nil)
	intent, _ := service.CreateIntent("бессонница")
	_, _ = service.ConfirmIntent(intent.ID, []string{"sleep"}, nil, nil)
	slots, _ := service.Slots("spec-lebedeva")
	hold, err := service.AcquireHold(intent.ID, slots[0].ID, intent.ClientIdentityID)
	if err != nil {
		t.Fatal(err)
	}
	instruction, err := service.CreateCheckout(hold.ID, intent.ClientIdentityID, "BANK_CARD")
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := service.ApplyProviderEvent(ProviderEvent{
		ProviderID: instruction.ProviderID, ProviderEventID: "evt-join", OrderID: instruction.OrderID,
		AmountMinor: instruction.AmountMinor, Currency: instruction.Currency, Outcome: "CAPTURED",
	})
	if err != nil {
		t.Fatal(err)
	}
	notice, join, err := service.Fulfillment(evidence.BookingID, intent.ClientIdentityID)
	if err != nil {
		t.Fatal(err)
	}
	if notice == nil || !notice.Transactional || notice.RequiresGrowthOptIn || notice.Purpose != "BOOKING_CONFIRMED" {
		t.Fatalf("notice = %#v", notice)
	}
	if join.Decision != "DENY" || join.ReasonCode != "COMM_OUTSIDE_JOIN_WINDOW" {
		t.Fatalf("join = %#v", join)
	}
	strangerNotice, stranger, err := service.Fulfillment(evidence.BookingID, "idn-stranger")
	if err != nil || stranger == nil || stranger.ReasonCode != "COMM_ROLE_MISMATCH" || strangerNotice != nil {
		t.Fatalf("foreign identity must not receive the booking notice: notice=%#v stranger=%#v err=%v", strangerNotice, stranger, err)
	}
	if _, err := service.CancelOrder(instruction.OrderID, "CLIENT_CANCEL"); err != nil {
		t.Fatal(err)
	}
	_, closed, err := service.Fulfillment(evidence.BookingID, intent.ClientIdentityID)
	if err != nil || closed.ReasonCode != "COMM_BOOKING_NOT_JOINABLE" {
		t.Fatalf("cancelled join = %#v err=%v", closed, err)
	}
}

func TestFulfillmentRefreshesBookingChangedByAnotherInstance(t *testing.T) {
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
		t.Fatalf("slots=%#v err=%v", slots, err)
	}
	hold, err := service.AcquireHold(intent.ID, slots[0].ID, intent.ClientIdentityID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateCheckout(hold.ID, intent.ClientIdentityID, "SBP"); err != nil {
		t.Fatal(err)
	}
	// Another process commits payment capture, then cancellation. This
	// instance must not authorize joins from its old PENDING_PAYMENT cache.
	for _, state := range []booking.State{booking.StateConfirmed, booking.StateCancelled} {
		for _, booked := range store.snapshot.Bookings {
			if booked != nil && booked.ID == hold.BookingID {
				booked.State = state
			}
		}
		_, join, err := service.Fulfillment(hold.BookingID, intent.ClientIdentityID)
		if err != nil || join == nil {
			t.Fatalf("refresh state %s: join=%#v err=%v", state, join, err)
		}
		if got := service.bookings[hold.BookingID].State; got != state {
			t.Fatalf("stale booking after refresh got=%s want=%s", got, state)
		}
		if state == booking.StateCancelled && join.ReasonCode != "COMM_BOOKING_NOT_JOINABLE" {
			t.Fatalf("cancelled remote booking must deny join: %#v", join)
		}
	}
}
