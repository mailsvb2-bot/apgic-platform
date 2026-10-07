package demand

import (
	"errors"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

func TestConformanceOfferKeepsOrganizationOwnerAndSpecialistAuthorExplicit(t *testing.T) {
	productContext, err := productContextForSpecialist("spec-lebedeva", "identity-spec-lebedeva")
	if err != nil {
		t.Fatal(err)
	}
	expectedOrganizationID, err := persistentid.FromRef("catalog-specialist-organization", "spec-lebedeva")
	if err != nil {
		t.Fatal(err)
	}
	expectedDirectionID, err := persistentid.FromRef("catalog-specialist-direction", "spec-lebedeva:consultation")
	if err != nil {
		t.Fatal(err)
	}
	expectedProductID, err := persistentid.FromRef("catalog-specialist-product", "spec-lebedeva")
	if err != nil {
		t.Fatal(err)
	}
	expectedOwnerRef := "organization/" + expectedOrganizationID
	if productContext.ProductID != expectedProductID ||
		productContext.OrganizationDirectionID != expectedDirectionID ||
		productContext.Ownership.OwnerType != "ORGANIZATION" ||
		productContext.Ownership.OwnerID != expectedOrganizationID ||
		productContext.Ownership.CommercialOwnerRef != expectedOwnerRef ||
		len(productContext.Ownership.AuthorRefs) != 1 ||
		productContext.Ownership.AuthorRefs[0] != "identity-spec-lebedeva" ||
		productContext.Ownership.RevenueBeneficiaryRef != "identity-spec-lebedeva" {
		t.Fatalf("product context = %#v", productContext)
	}
}

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
	storedHold := service.holds[hold.ID]
	if storedHold == nil || storedHold.State != "CONSUMED" || storedHold.BookingState != "PENDING_PAYMENT" {
		t.Fatalf("checkout must consume hold and create pending booking: %#v", storedHold)
	}
	booked := service.bookings[hold.BookingID]
	if booked == nil || booked.State != "PENDING_PAYMENT" || booked.HoldID != hold.ID {
		t.Fatalf("booking = %#v", booked)
	}
	again, err := service.CreateCheckout(hold.ID, intent.ClientIdentityID, "BANK_CARD")
	if err != nil || again.ID != instruction.ID || again.AmountMinor != instruction.AmountMinor {
		t.Fatalf("idempotent instruction = %#v err=%v", again, err)
	}
	if _, err := service.CreateCheckout(hold.ID, intent.ClientIdentityID, "SBP"); !errors.Is(err, ErrCheckoutLocked) {
		t.Fatalf("method change err = %v", err)
	}
	if _, err := service.CreateCheckout(hold.ID, "idn-stranger", "BANK_CARD"); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("foreign replay err = %v", err)
	}
}

func TestLiveBookingBlocksSecondHoldUntilPaymentTimeoutExpires(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	service := NewConformanceService(func() time.Time { return now })
	first, _ := service.CreateIntent("нужна помощь со сном")
	second, _ := service.CreateIntent("тоже не сплю")
	_, _ = service.ConfirmIntent(first.ID, []string{"sleep"}, nil, nil)
	_, _ = service.ConfirmIntent(second.ID, []string{"sleep"}, nil, nil)
	slots, err := service.Slots("spec-lebedeva")
	if err != nil || len(slots) == 0 {
		t.Fatalf("slots=%#v err=%v", slots, err)
	}
	hold, err := service.AcquireHold(first.ID, slots[0].ID, first.ClientIdentityID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateCheckout(hold.ID, first.ClientIdentityID, "SBP"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AcquireHold(second.ID, slots[0].ID, second.ClientIdentityID); !errors.Is(err, ErrSlotBooked) {
		t.Fatalf("second hold while booking live err=%v", err)
	}

	now = hold.ExpiresAt.Add(time.Second)
	replacement, err := service.AcquireHold(second.ID, slots[0].ID, second.ClientIdentityID)
	if err != nil {
		t.Fatalf("replacement hold after timeout err=%v", err)
	}
	if replacement.State != "ACTIVE" {
		t.Fatalf("replacement=%#v", replacement)
	}
	if booked := service.bookings[hold.BookingID]; booked == nil || booked.State != "EXPIRED" {
		t.Fatalf("timed out booking=%#v", booked)
	}
	storedHold := service.holds[hold.ID]
	if storedHold == nil || storedHold.State != "CONSUMED" || storedHold.BookingState != "EXPIRED" {
		t.Fatalf("consumed hold after booking timeout=%#v", storedHold)
	}
	if _, err := service.CreateCheckout(hold.ID, first.ClientIdentityID, "SBP"); !errors.Is(err, ErrHoldNotActive) {
		t.Fatalf("expired checkout replay err=%v", err)
	}
}

func TestCheckoutReplayAllowedOnlyWhilePaymentPending(t *testing.T) {
	states := []booking.State{
		booking.StateHeld,
		booking.StateConfirmed,
		booking.StateCancelled,
		booking.StateExpired,
		booking.StateCompleted,
		booking.StateNoShow,
	}
	for _, state := range states {
		t.Run(string(state), func(t *testing.T) {
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
			booked := service.bookings[instruction.BookingID]
			booked.State = state
			if _, err := service.CreateCheckout(hold.ID, intent.ClientIdentityID, "SBP"); !errors.Is(err, ErrHoldNotActive) {
				t.Fatalf("state=%s replay err=%v", state, err)
			}
		})
	}
}

func TestCheckoutReplayUsesDurableInstructionAfterCatalogSlotRollsOut(t *testing.T) {
	now := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	holdExpiresAt := now.Add(10 * time.Minute)
	const (
		holdID    = "hold-persisted"
		bookingID = "booking-persisted"
		clientID  = "client-persisted"
		slotID    = "slot-no-longer-in-live-catalog"
	)

	store := &fakeJourneyStore{
		snapshot: JourneySnapshot{
			Holds: []*Hold{{
				ID:               holdID,
				BookingID:        bookingID,
				SlotID:           slotID,
				ClientIdentityID: clientID,
				State:            "CONSUMED",
				BookingState:     booking.StatePendingPayment,
				ExpiresAt:        holdExpiresAt,
				ReasonCode:       "BOOK_HOLD_ACQUIRED",
			}},
			Bookings: []*booking.Booking{{
				ID:               bookingID,
				SlotID:           slotID,
				HoldID:           holdID,
				ClientIdentityID: clientID,
				State:            booking.StatePendingPayment,
				HoldExpiresAt:    holdExpiresAt,
				StartsAt:         now.Add(time.Hour),
				EndsAt:           now.Add(2 * time.Hour),
				UpdatedAt:        now,
			}},
			Instructions: []*CheckoutInstruction{{
				ID:                 "checkout-persisted",
				HoldID:             holdID,
				BookingID:          bookingID,
				BookingState:       booking.StatePendingPayment,
				OrderID:            "order-persisted",
				ProviderID:         "external-bank",
				MethodCode:         "SBP",
				RailCode:           "BANK_TRANSFER",
				AmountMinor:        450000,
				Currency:           "RUB",
				ExecutionOwner:     "EXTERNAL_PROVIDER",
				PaymentRecipientID: "identity-spec-lebedeva",
				PlatformRole:       platformRole,
				APGICAcceptsFunds:  false,
				ReasonCode:         "PAY_ROUTE_SELECTED",
			}},
		},
	}

	service, err := NewConformanceServiceWithStores(func() time.Time { return now }, nil, store)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := service.slot(slotID); ok {
		t.Fatalf("regression setup invalid: retired slot %q unexpectedly exists in live catalog", slotID)
	}

	replayed, err := service.CreateCheckout(holdID, clientID, "SBP")
	if err != nil {
		t.Fatalf("durable checkout replay must not depend on live catalog slot: %v", err)
	}
	if replayed.ID != "checkout-persisted" || replayed.OrderID != "order-persisted" {
		t.Fatalf("replayed instruction=%#v", replayed)
	}
	if _, err := service.CreateCheckout(holdID, clientID, "BANK_CARD"); !errors.Is(err, ErrCheckoutLocked) {
		t.Fatalf("method change after durable replay err=%v", err)
	}
	if _, err := service.CreateCheckout(holdID, "foreign-client", "SBP"); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("foreign durable replay err=%v", err)
	}
}
