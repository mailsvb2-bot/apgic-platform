package demand

import (
	"errors"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"
)

// A corrupted CONFIRMED flag without the captured ledger is not a
// consultation entitlement. Cancellation also revokes preexisting sessions.
func TestConsultationRequiresRealPaymentEvidenceNotConfirmedBookingFlag(t *testing.T) {
	now := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)
	service := NewConformanceService(func() time.Time { return now })
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
	instruction, err := service.CreateCheckout(hold.ID, intent.ClientIdentityID, "SBP")
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately simulate an inconsistent booking write with no payment.
	service.bookings[hold.BookingID].State = booking.StateConfirmed
	now = slots[0].StartsAt.Add(-10 * time.Minute)
	_, join, err := service.Fulfillment(hold.BookingID, intent.ClientIdentityID)
	if err != nil || join == nil || join.Decision != "DENY" || join.ReasonCode != "COMM_ENTITLEMENT_REQUIRED" {
		t.Fatalf("unpaid CONFIRMED booking opened media join: %#v err=%v", join, err)
	}
	if view, err := service.RecordSessionPresence(hold.BookingID); !errors.Is(err, ErrConsultNotReady) || view != nil {
		t.Fatalf("unpaid booking started session: %#v err=%v", view, err)
	}
	// Capture must occur before the original hold expiry.
	now = time.Date(2026, 10, 12, 8, 1, 0, 0, time.UTC)
	service.bookings[hold.BookingID].State = booking.StatePendingPayment
	_, err = service.ApplyProviderEvent(ProviderEvent{
		ProviderID: instruction.ProviderID, ProviderEventID: "evt-paid-gate",
		OrderID: instruction.OrderID, AmountMinor: instruction.AmountMinor,
		Currency: instruction.Currency, Outcome: "CAPTURED",
	})
	if err != nil {
		t.Fatal(err)
	}
	now = slots[0].StartsAt.Add(-10 * time.Minute)
	_, join, err = service.Fulfillment(hold.BookingID, intent.ClientIdentityID)
	if err != nil || join == nil || join.Decision != "ALLOW" {
		t.Fatalf("verified capture could not join: %#v err=%v", join, err)
	}
	if _, err := service.RecordSessionPresence(hold.BookingID); err != nil {
		t.Fatalf("paid conformance session not admitted: %v", err)
	}
	if _, err := service.CancelOrderForClient(instruction.OrderID, intent.ClientIdentityID, "CLIENT_CANCEL"); err != nil {
		t.Fatal(err)
	}
	_, join, err = service.Fulfillment(hold.BookingID, intent.ClientIdentityID)
	if err != nil || join == nil || join.Decision != "DENY" {
		t.Fatalf("cancelled booking still admitted: %#v err=%v", join, err)
	}
	if view, err := service.RecordSessionPresence(hold.BookingID); !errors.Is(err, ErrConsultNotReady) || view != nil {
		t.Fatalf("previous session survived cancellation: %#v err=%v", view, err)
	}
}
