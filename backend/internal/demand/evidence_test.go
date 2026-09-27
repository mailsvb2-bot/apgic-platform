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
	reconciliation, err := service.LedgerReconciliation()
	if err != nil {
		t.Fatal(err)
	}
	if reconciliation.EntryCount != 1 || len(reconciliation.CurrencyTotals) != 1 {
		t.Fatalf("capture ledger reconciliation = %#v", reconciliation)
	}
	totals := reconciliation.CurrencyTotals[0]
	if totals.Currency != instruction.Currency ||
		totals.DebitMinor != instruction.AmountMinor ||
		totals.CreditMinor != instruction.AmountMinor {
		t.Fatalf("capture ledger totals = %#v", totals)
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


func TestProviderCaptureFailsClosedWhenLedgerPersistenceFails(t *testing.T) {
	store := &fakeLedgerStore{appendErr: errors.New("ledger db unavailable")}
	service := NewConformanceServiceWithLedgerStore(nil, store)
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
	booked := service.bookings[instruction.BookingID]
	if booked.State != "PENDING_PAYMENT" {
		t.Fatalf("precondition booking state = %s", booked.State)
	}

	_, err = service.ApplyProviderEvent(ProviderEvent{
		ProviderID: instruction.ProviderID, ProviderEventID: "evt-store-fail",
		OrderID: instruction.OrderID, AmountMinor: instruction.AmountMinor,
		Currency: instruction.Currency, Outcome: "CAPTURED",
	})
	if err == nil {
		t.Fatal("capture must fail when durable ledger append fails")
	}
	if booked.State != "PENDING_PAYMENT" {
		t.Fatalf("failed ledger append mutated booking state to %s", booked.State)
	}
	if len(store.entries) != 0 || len(service.evidence) != 0 || len(service.orderEvidence) != 0 {
		t.Fatalf("failed ledger append committed business state: entries=%d evidence=%d orderEvidence=%d",
			len(store.entries), len(service.evidence), len(service.orderEvidence))
	}
}
