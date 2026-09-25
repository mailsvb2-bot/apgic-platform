package demand

import (
	"errors"
	"testing"
)

func TestCheckoutSendsMoneyToExternalProviderAndSpecialist(t *testing.T) {
	service := NewConformanceService(nil)
	intent, err := service.CreateIntent("нужна помощь со сном")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmIntent(intent.ID, []string{"sleep"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	slots, err := service.Slots("spec-lebedeva")
	if err != nil {
		t.Fatal(err)
	}
	hold, err := service.AcquireHold(intent.ID, slots[0].ID, intent.ClientIdentityID)
	if err != nil {
		t.Fatal(err)
	}
	options, err := service.CheckoutOptions(hold.ID, intent.ClientIdentityID)
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 2 || options[0].APGICAcceptsFunds || options[0].ExecutionOwner != "EXTERNAL_PROVIDER" {
		t.Fatalf("options = %#v", options)
	}
	for _, option := range options {
		if option.MethodCode == "WALLET" || option.PaymentRecipientID != "identity-spec-lebedeva" {
			t.Fatalf("option points at the wrong recipient: %#v", option)
		}
	}
	if _, err := service.CreateCheckout(hold.ID, intent.ClientIdentityID, "WALLET"); !errors.Is(err, ErrMethodNotEligible) {
		t.Fatalf("wallet err = %v", err)
	}
	instruction, err := service.CreateCheckout(hold.ID, intent.ClientIdentityID, "BANK_CARD")
	if err != nil {
		t.Fatal(err)
	}
	if instruction.APGICAcceptsFunds || instruction.ExecutionOwner != "EXTERNAL_PROVIDER" || instruction.BookingState != "PENDING_PAYMENT" {
		t.Fatalf("instruction = %#v", instruction)
	}
	if instruction.PaymentRecipientID != "identity-spec-lebedeva" || instruction.ProviderID != "external-bank" {
		t.Fatalf("funds owner = %#v", instruction)
	}
	again, err := service.CreateCheckout(hold.ID, intent.ClientIdentityID, "BANK_CARD")
	if err != nil || again.ID != instruction.ID || again.AmountMinor != instruction.AmountMinor {
		t.Fatalf("idempotent instruction = %#v err=%v", again, err)
	}
	if _, err := service.CreateCheckout(hold.ID, intent.ClientIdentityID, "SBP"); !errors.Is(err, ErrCheckoutLocked) {
		t.Fatalf("method change err = %v", err)
	}
}
