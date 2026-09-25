package demand

import (
	"errors"
	"testing"
)

func TestAccountDeletionKeepsTheLedgerAndIsNotDeactivation(t *testing.T) {
	service := NewConformanceService(nil)
	intent, err := service.CreateIntent("бессонница")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmIntent(intent.ID, []string{"sleep"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	slots, _ := service.Slots("spec-lebedeva")
	hold, err := service.AcquireHold(intent.ID, slots[2].ID, intent.ClientIdentityID)
	if err != nil {
		t.Fatal(err)
	}
	instruction, err := service.CreateCheckout(hold.ID, intent.ClientIdentityID, "BANK_CARD")
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := service.ApplyProviderEvent(ProviderEvent{
		ProviderID: instruction.ProviderID, ProviderEventID: "evt-delete", OrderID: instruction.OrderID,
		AmountMinor: instruction.AmountMinor, Currency: instruction.Currency, Outcome: "CAPTURED",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.DeleteAccount(intent.ClientIdentityID, "DEACTIVATION"); !errors.Is(err, ErrNotDeletion) {
		t.Fatalf("deactivation err = %v", err)
	}
	deleted, err := service.DeleteAccount(intent.ClientIdentityID, "WEB")
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Deactivation || deleted.APGICDeletesLedger || !deleted.LedgerRetained || !deleted.ProfileErased {
		t.Fatalf("deletion = %#v", deleted)
	}
	if deleted.State != "PARTIALLY_RETAINED_WITH_REASON" || deleted.LedgerID != evidence.LedgerEntryID {
		t.Fatalf("retained ledger = %#v evidence=%s", deleted, evidence.LedgerEntryID)
	}
	if service.bookings[evidence.BookingID] == nil {
		t.Fatal("booking was destroyed")
	}
	if service.evidence[instruction.ProviderID+"/evt-delete"] == nil {
		t.Fatal("provider evidence was destroyed")
	}
	again, err := service.DeleteAccount(intent.ClientIdentityID, "WEB")
	if err != nil || !again.Idempotent || again.ID != deleted.ID || again.LedgerID != deleted.LedgerID {
		t.Fatalf("replay = %#v err=%v", again, err)
	}
}
