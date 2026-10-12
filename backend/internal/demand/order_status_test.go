package demand

import (
	"errors"
	"testing"
)

func TestOrderReadbackOwnerPaymentAndCancellationAreDistinct(t *testing.T) {
	service := NewConformanceService(nil)
	intent, err := service.CreateIntent("бессонница")
	if err != nil { t.Fatal(err) }
	if _, err := service.ConfirmIntent(intent.ID, []string{"sleep"}, nil, nil); err != nil { t.Fatal(err) }
	slots, err := service.Slots("spec-lebedeva")
	if err != nil || len(slots) == 0 { t.Fatalf("slots=%v err=%v", slots, err) }
	hold, err := service.AcquireHold(intent.ID, slots[0].ID, intent.ClientIdentityID)
	if err != nil { t.Fatal(err) }
	instruction, err := service.CreateCheckout(hold.ID, intent.ClientIdentityID, "SBP")
	if err != nil { t.Fatal(err) }
	read := func() *OrderStatus {
		t.Helper()
		state, err := service.ReadOrderStatus(instruction.OrderID, intent.ClientIdentityID)
		if err != nil { t.Fatal(err) }
		return state
	}
	if s := read(); s.PaymentState != "PENDING_PROVIDER" || s.BookingState != "PENDING_PAYMENT" || s.RefundState != "" {
		t.Fatalf("premature payment success: %#v", s)
	}
	for _, identity := range []string{"", "other-client"} {
		if s, err := service.ReadOrderStatus(instruction.OrderID, identity); !errors.Is(err, ErrOrderNotFound) || s != nil {
			t.Fatalf("foreign disclosure: %#v %v", s, err)
		}
	}
	_, err = service.ApplyProviderEvent(ProviderEvent{
		ProviderID: instruction.ProviderID, ProviderEventID: "evt-readback",
		OrderID: instruction.OrderID, AmountMinor: instruction.AmountMinor,
		Currency: instruction.Currency, Outcome: "CAPTURED",
	})
	if err != nil { t.Fatal(err) }
	if s := read(); s.PaymentState != "CAPTURED" || s.BookingState != "CONFIRMED" { t.Fatalf("capture: %#v", s) }
	if c, err := service.CancelOrderForClient(instruction.OrderID, "other-client", "CLIENT_CANCEL"); !errors.Is(err, ErrOrderNotFound) || c != nil {
		t.Fatalf("foreign cancellation: %#v %v", c, err)
	}
	if read().BookingState != "CONFIRMED" { t.Fatal("foreign actor cancelled booking") }
	c, err := service.CancelOrderForClient(instruction.OrderID, intent.ClientIdentityID, "CLIENT_CANCEL")
	if err != nil { t.Fatal(err) }
	if s := read(); s.BookingState != "CANCELLED" || s.PaymentState != "CAPTURED" || s.RefundState != c.RefundState {
		t.Fatalf("post cancel: %#v", s)
	}
}
