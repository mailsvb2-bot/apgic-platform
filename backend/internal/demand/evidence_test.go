package demand

import (
	"errors"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"
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
	for _, mismatch := range []struct {
		name string
		edit func(*ProviderEvent)
	}{
		{"foreign_order", func(e *ProviderEvent) { e.OrderID = "other-order" }},
		{"changed_amount", func(e *ProviderEvent) { e.AmountMinor++ }},
		{"changed_currency", func(e *ProviderEvent) { e.Currency = "USD" }},
	} {
		t.Run(mismatch.name, func(t *testing.T) {
			conflicting := event
			mismatch.edit(&conflicting)
			if replay, err := service.ApplyProviderEvent(conflicting); !errors.Is(err, ErrEvidenceMismatch) || replay != nil {
				t.Fatalf("conflicting provider replay=%#v err=%v", replay, err)
			}
		})
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

func TestProviderCaptureAfterHoldExpiryCreatesNoEconomicEffect(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	service := NewConformanceService(func() time.Time { return now })
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

	now = hold.ExpiresAt.Add(time.Second)
	_, err = service.ApplyProviderEvent(ProviderEvent{
		ProviderID: instruction.ProviderID, ProviderEventID: "evt-after-expiry",
		OrderID: instruction.OrderID, AmountMinor: instruction.AmountMinor,
		Currency: instruction.Currency, Outcome: "CAPTURED",
	})
	if !errors.Is(err, booking.ErrHoldExpired) {
		t.Fatalf("capture after expiry err=%v", err)
	}
	booked := service.bookings[instruction.BookingID]
	if booked == nil || booked.State != booking.StatePendingPayment {
		t.Fatalf("expired capture mutated booking=%#v", booked)
	}
	reconciliation, err := service.LedgerReconciliation()
	if err != nil {
		t.Fatal(err)
	}
	if reconciliation.EntryCount != 0 || len(service.evidence) != 0 || len(service.orderEvidence) != 0 {
		t.Fatalf("expired capture created effect: reconciliation=%#v evidence=%d orderEvidence=%d",
			reconciliation, len(service.evidence), len(service.orderEvidence))
	}
}
