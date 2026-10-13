package demand

import (
	"errors"
	"testing"
	"time"

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
	if !errors.Is(err, ErrOrderNotFound) || stranger != nil || strangerNotice != nil {
		t.Fatalf("foreign identity learned booking details: notice=%#v join=%#v err=%v", strangerNotice, stranger, err)
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

func TestConsultationRefusesStaleConfirmedBookingAfterRemoteCancellation(t *testing.T) {
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
	// The current API instance last saw a confirmed booking, but another
	// instance has already cancelled it in PostgreSQL.
	service.bookings[hold.BookingID].State = booking.StateConfirmed
	for _, booked := range store.snapshot.Bookings {
		if booked != nil && booked.ID == hold.BookingID {
			booked.State = booking.StateCancelled
		}
	}
	if view, err := service.RecordSessionPresence(hold.BookingID); !errors.Is(err, ErrConsultNotReady) || view != nil {
		t.Fatalf("stale confirmed booking started consultation: view=%#v err=%v", view, err)
	}
	if _, ok := service.sessions[hold.BookingID]; ok {
		t.Fatal("stale confirmation created a consultation session")
	}
}

// This is a separate production/conformance boundary test, not merely an
// HTTP flag check: no caller may bypass the transport guard by invoking the
// service directly after live supply has been selected.
func TestLiveBookingsDoNotMintConformanceCommunicationCredentials(t *testing.T) {
	now := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)
	service := NewConformanceService(func() time.Time { return now })
	intent, err := service.CreateIntent("бессонница")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ConfirmIntent(intent.ID, []string{"sleep"}, nil, nil); err != nil {
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
	instruction, err := service.CreateCheckout(hold.ID, intent.ClientIdentityID, "SBP")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ApplyProviderEvent(ProviderEvent{
		ProviderID: instruction.ProviderID, ProviderEventID: "verified-before-live",
		OrderID: instruction.OrderID, AmountMinor: instruction.AmountMinor,
		Currency: instruction.Currency, Outcome: "CAPTURED",
	}); err != nil {
		t.Fatal(err)
	}
	// Even a captured booking inside the permitted join window must not be
	// handed the fake 'comm-external' provider as a real credential issuer.
	now = slots[0].StartsAt.Add(-10 * time.Minute)
	service.liveSupplyMode = true
	notice, join, err := service.Fulfillment(hold.BookingID, intent.ClientIdentityID)
	if err != nil || join == nil || join.Decision != "DENY" ||
		join.ReasonCode != "COMM_PROVIDER_UNAVAILABLE" || join.ProviderInstanceID != "" ||
		notice == nil {
		t.Fatalf("live service leaked conformance join: notice=%#v join=%#v err=%v", notice, join, err)
	}
	if view, err := service.RecordSessionPresence(hold.BookingID); !errors.Is(err, ErrConsultNotReady) || view != nil {
		t.Fatalf("live service created synthetic session: %#v err=%v", view, err)
	}
	if view, err := service.CompleteSession(hold.BookingID, "injected-provider-evidence"); !errors.Is(err, ErrConsultNotReady) || view != nil {
		t.Fatalf("live service completed synthetic session: %#v err=%v", view, err)
	}
	if view, err := service.ReportProviderFailure(hold.BookingID, "ROOM_FAILURE", "injected-provider-evidence", false); !errors.Is(err, ErrConsultNotReady) || view != nil {
		t.Fatalf("live service simulated a provider refund: %#v err=%v", view, err)
	}
	// Read paths must not disclose timing, provider identity or notice.
	for _, foreign := range []string{"", "some-other-client"} {
		if notice, join, err := service.Fulfillment(hold.BookingID, foreign); !errors.Is(err, ErrOrderNotFound) || notice != nil || join != nil {
			t.Fatalf("live foreign client leaked booking: notice=%#v join=%#v err=%v", notice, join, err)
		}
	}
}
