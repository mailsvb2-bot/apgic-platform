package demand

import (
	"errors"
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
	reconciliation, err := service.LedgerReconciliation()
	if err != nil {
		t.Fatal(err)
	}
	if reconciliation.EntryCount != 2 || len(reconciliation.CurrencyTotals) != 1 {
		t.Fatalf("cancel ledger reconciliation = %#v", reconciliation)
	}
	for _, balance := range reconciliation.Balances {
		if balance.AmountMinor != 0 {
			t.Fatalf("capture + reversal must net account balance to zero: %#v", reconciliation.Balances)
		}
	}
	totals := reconciliation.CurrencyTotals[0]
	if totals.DebitMinor != evidence.AmountMinor*2 ||
		totals.CreditMinor != evidence.AmountMinor*2 {
		t.Fatalf("capture + reversal totals = %#v", totals)
	}
}


func TestCancellationFailsClosedWhenLedgerPersistenceFails(t *testing.T) {
	store := &fakeLedgerStore{}
	service := NewConformanceServiceWithLedgerStore(nil, store)
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
	_, err = service.ApplyProviderEvent(ProviderEvent{
		ProviderID: instruction.ProviderID, ProviderEventID: "evt-cancel-store-fail",
		OrderID: instruction.OrderID, AmountMinor: instruction.AmountMinor,
		Currency: instruction.Currency, Outcome: "CAPTURED",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(store.entries) != 1 {
		t.Fatalf("capture must persist one ledger entry, got %d", len(store.entries))
	}

	store.appendErr = errors.New("ledger db unavailable")
	booked := service.bookings[instruction.BookingID]
	if booked.State != "CONFIRMED" {
		t.Fatalf("precondition booking state = %s", booked.State)
	}
	if _, err := service.CancelOrder(instruction.OrderID, "CLIENT_CANCEL"); err == nil {
		t.Fatal("cancellation must fail when durable ledger append fails")
	}
	if booked.State != "CONFIRMED" {
		t.Fatalf("failed reversal append mutated booking state to %s", booked.State)
	}
	if len(store.entries) != 1 || len(service.reversals) != 0 {
		t.Fatalf("failed reversal append committed state: entries=%d reversals=%d",
			len(store.entries), len(service.reversals))
	}
}
