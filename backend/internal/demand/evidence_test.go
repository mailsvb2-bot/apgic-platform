package demand

import (
	"errors"
	"testing"
)

func TestProviderCaptureConfirmsOnceAndDoesNotPayAPGIC(t *testing.T) {
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
	event := ProviderEvent{
		ProviderID:      instruction.ProviderID,
		ProviderEventID: "evt-1",
		OrderID:         instruction.OrderID,
		AmountMinor:     instruction.AmountMinor,
		Currency:        instruction.Currency,
		Outcome:         "CAPTURED",
	}
	first, err := service.ApplyProviderEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	if first.Idempotent || first.APGICAcceptsFunds || first.BookingState != "CONFIRMED" {
		t.Fatalf("first = %#v", first)
	}
	if first.CreditAccountRef != "identity-spec-lebedeva" || first.DebitAccountRef != "external-provider/external-bank/settlement" {
		t.Fatalf("accounts = %#v", first)
	}
	second, err := service.ApplyProviderEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Idempotent || second.LedgerEntryID != first.LedgerEntryID || second.ID != first.ID {
		t.Fatalf("replay = %#v", second)
	}
	if _, err := service.ApplyProviderEvent(ProviderEvent{
		ProviderID:      instruction.ProviderID,
		ProviderEventID: "evt-2",
		OrderID:         instruction.OrderID,
		AmountMinor:     instruction.AmountMinor,
		Currency:        instruction.Currency,
		Outcome:         "CAPTURED",
	}); !errors.Is(err, ErrDuplicateEffect) {
		t.Fatalf("second effect err = %v", err)
	}
	event.AmountMinor++
	event.ProviderEventID = "evt-3"
	if _, err := service.ApplyProviderEvent(event); !errors.Is(err, ErrEvidenceMismatch) {
		t.Fatalf("mismatch err = %v", err)
	}
}
