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
	if conflicting, err := service.CompleteSession(evidence.BookingID, "other-provider-end"); !errors.Is(err, ErrConsultEvidence) || conflicting != nil {
		t.Fatalf("conflicting completion evidence must fail closed: view=%#v err=%v", conflicting, err)
	}
	if again, err := service.CompleteSession(evidence.BookingID, "provider-room-end"); err != nil || !again.Idempotent || again.EvidenceRef != "provider-room-end" {
		t.Fatalf("original evidence must remain replayable: view=%#v err=%v", again, err)
	}
	presenceAgain, err := service.RecordSessionPresence(evidence.BookingID)
	if err != nil || presenceAgain.State != "COMPLETED" || !presenceAgain.Idempotent || presenceAgain.EvidenceRef != "provider-room-end" {
		t.Fatalf("completed presence replay lost original provider evidence: view=%#v err=%v", presenceAgain, err)
	}
}
