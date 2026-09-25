package demand

import (
	"errors"
	"testing"
)

func confirmedSession(t *testing.T) (*Service, string) {
	t.Helper()
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
		ProviderID: instruction.ProviderID, ProviderEventID: "evt-recovery", OrderID: instruction.OrderID,
		AmountMinor: instruction.AmountMinor, Currency: instruction.Currency, Outcome: "CAPTURED",
	})
	if err != nil {
		t.Fatal(err)
	}
	presence, err := service.RecordSessionPresence(evidence.BookingID)
	if err != nil || presence.State != "IN_PROGRESS" {
		t.Fatalf("presence = %#v err=%v", presence, err)
	}
	return service, evidence.BookingID
}

func TestProviderFailureRecoversWithoutChargeOrCompletion(t *testing.T) {
	service, bookingID := confirmedSession(t)
	failed, err := service.ReportProviderFailure(bookingID, "PROVIDER_DISCONNECT", "provider-evidence/disconnect-1", true)
	if err != nil {
		t.Fatal(err)
	}
	if failed.State != "RECOVERING" || failed.ChargedAgain || failed.RefundPathOpened || failed.RecoveryAction != "FALLBACK_PROVIDER" {
		t.Fatalf("failure = %#v", failed)
	}
	if failed.ProviderID != "comm-external-fallback" {
		t.Fatalf("provider = %s", failed.ProviderID)
	}
	if _, err := service.CompleteSession(bookingID, ""); !errors.Is(err, ErrConsultEvidence) {
		t.Fatalf("empty completion err = %v", err)
	}
	restored, err := service.SucceedRecovery(bookingID, "provider-evidence/recovered-1")
	if err != nil || restored.State != "IN_PROGRESS" || restored.ChargedAgain {
		t.Fatalf("restored = %#v err=%v", restored, err)
	}
	replay, err := service.ReportProviderFailure(bookingID, "PROVIDER_DISCONNECT", "provider-evidence/disconnect-1", true)
	if err != nil || replay.State != "RECOVERING" || replay.ChargedAgain {
		t.Fatalf("second failure = %#v err=%v", replay, err)
	}
}

func TestUnrecoverableFailureOpensExternalRefundAndDoesNotComplete(t *testing.T) {
	service, bookingID := confirmedSession(t)
	failed, err := service.ReportProviderFailure(bookingID, "ROOM_FAILURE", "provider-evidence/room-1", false)
	if err != nil {
		t.Fatal(err)
	}
	if failed.State != "TECHNICAL_FAILURE" || !failed.RefundPathOpened || failed.ChargedAgain || failed.APGICReturnsFunds {
		t.Fatalf("exhausted = %#v", failed)
	}
	if failed.RecoveryAction != "REFUND" {
		t.Fatalf("action = %s", failed.RecoveryAction)
	}
	if _, err := service.CompleteSession(bookingID, "provider-room-end"); !errors.Is(err, ErrConsultNotReady) {
		t.Fatalf("false completion err = %v", err)
	}
	again, err := service.ReportProviderFailure(bookingID, "ROOM_FAILURE", "provider-evidence/room-1", false)
	if err != nil || !again.Idempotent || again.State != "TECHNICAL_FAILURE" || again.APGICReturnsFunds {
		t.Fatalf("replay = %#v err=%v", again, err)
	}
}

func TestFailureBeforePresenceIsRejected(t *testing.T) {
	service := NewConformanceService(nil)
	intent, _ := service.CreateIntent("бессонница")
	_, _ = service.ConfirmIntent(intent.ID, []string{"sleep"}, nil, nil)
	slots, _ := service.Slots("spec-lebedeva")
	hold, _ := service.AcquireHold(intent.ID, slots[1].ID, intent.ClientIdentityID)
	instruction, _ := service.CreateCheckout(hold.ID, intent.ClientIdentityID, "SBP")
	evidence, err := service.ApplyProviderEvent(ProviderEvent{
		ProviderID: instruction.ProviderID, ProviderEventID: "evt-early", OrderID: instruction.OrderID,
		AmountMinor: instruction.AmountMinor, Currency: instruction.Currency, Outcome: "CAPTURED",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReportProviderFailure(evidence.BookingID, "NETWORK_LOSS", "provider-evidence/net-1", true); !errors.Is(err, ErrConsultNotReady) {
		t.Fatalf("early failure err = %v", err)
	}
	if _, err := service.ReportProviderFailure(evidence.BookingID, "NETWORK_LOSS", "", true); !errors.Is(err, ErrConsultEvidence) {
		t.Fatalf("missing evidence err = %v", err)
	}
}
