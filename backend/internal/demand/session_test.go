package demand

import (
	"errors"
	"testing"
)

func TestConsultationCompletesOnlyWithProviderEvidenceAndDoesNotChargeAgain(t *testing.T) {
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
		ProviderID: instruction.ProviderID, ProviderEventID: "evt-session", OrderID: instruction.OrderID,
		AmountMinor: instruction.AmountMinor, Currency: instruction.Currency, Outcome: "CAPTURED",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CompleteSession(evidence.BookingID, ""); !errors.Is(err, ErrConsultEvidence) {
		t.Fatalf("empty evidence err = %v", err)
	}
	if _, err := service.CompleteSession(evidence.BookingID, "provider-room-end"); !errors.Is(err, ErrConsultNotReady) {
		t.Fatalf("early complete err = %v", err)
	}
	presence, err := service.RecordSessionPresence(evidence.BookingID)
	if err != nil || presence.State != "IN_PROGRESS" || presence.ChargedAgain || presence.APGICOwnsRoom {
		t.Fatalf("presence = %#v err=%v", presence, err)
	}
	done, err := service.CompleteSession(evidence.BookingID, "provider-room-end")
	if err != nil || done.State != "COMPLETED" || done.EvidenceRef != "provider-room-end" || done.ChargedAgain {
		t.Fatalf("complete = %#v err=%v", done, err)
	}
	again, err := service.CompleteSession(evidence.BookingID, "provider-room-end")
	if err != nil || !again.Idempotent || again.ID != done.ID {
		t.Fatalf("replay = %#v err=%v", again, err)
	}
}
