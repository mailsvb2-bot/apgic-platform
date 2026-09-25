package demand

import (
	"testing"
)

func TestCancellationReversesThroughOriginalProviderAndKeepsPayment(t *testing.T) {
	service := NewConformanceService(nil)
	intent, _ := service.CreateIntent("бессонница")
	_, _ = service.ConfirmIntent(intent.ID, []string{"sleep"}, nil, nil)
	slots, _ := service.Slots("spec-lebedeva")
	hold, err := service.AcquireHold(intent.ID, slots[0].ID, intent.ClientIdentityID)
	if err != nil {
		t.Fatal(err)
	}
	instruction, err := service.CreateCheckout(hold.ID, intent.ClientIdentityID, "SBP")
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := service.ApplyProviderEvent(ProviderEvent{
		ProviderID:      instruction.ProviderID,
		ProviderEventID: "evt-cancel",
		OrderID:         instruction.OrderID,
		AmountMinor:     instruction.AmountMinor,
		Currency:        instruction.Currency,
		Outcome:         "CAPTURED",
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.CancelOrder(instruction.OrderID, "CLIENT_CANCEL")
	if err != nil {
		t.Fatal(err)
	}
	if first.BookingState != "CANCELLED" || first.RefundState != "SUCCEEDED" || first.APGICReturnsFunds || first.APGICAcceptsFunds {
		t.Fatalf("cancellation = %#v", first)
	}
	if first.ProviderID != "external-bank" || first.ExecutionOwner != "EXTERNAL_PROVIDER" {
		t.Fatalf("provider = %#v", first)
	}
	if first.OriginalLedgerID != evidence.LedgerEntryID || first.ReversalLedgerID == evidence.LedgerEntryID {
		t.Fatalf("ledger history = %#v", first)
	}
	second, err := service.CancelOrder(instruction.OrderID, "CLIENT_CANCEL")
	if err != nil || !second.Idempotent || second.RefundID != first.RefundID {
		t.Fatalf("replay = %#v err=%v", second, err)
	}
	if _, ok := service.evidence[instruction.ProviderID+"/evt-cancel"]; !ok {
		t.Fatal("original provider capture was deleted")
	}
}
