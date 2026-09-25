package demand

import "testing"

func TestConfirmedBookingNotifiesWithoutMarketingOptInAndGatesJoin(t *testing.T) {
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
		ProviderID: instruction.ProviderID, ProviderEventID: "evt-join", OrderID: instruction.OrderID,
		AmountMinor: instruction.AmountMinor, Currency: instruction.Currency, Outcome: "CAPTURED",
	})
	if err != nil {
		t.Fatal(err)
	}
	notice, join, err := service.Fulfillment(evidence.BookingID, intent.ClientIdentityID)
	if err != nil {
		t.Fatal(err)
	}
	if notice == nil || !notice.Transactional || notice.RequiresGrowthOptIn || notice.Purpose != "BOOKING_CONFIRMED" {
		t.Fatalf("notice = %#v", notice)
	}
	if join.Decision != "DENY" || join.ReasonCode != "COMM_OUTSIDE_JOIN_WINDOW" {
		t.Fatalf("join = %#v", join)
	}
	_, stranger, err := service.Fulfillment(evidence.BookingID, "idn-stranger")
	if err != nil || stranger.ReasonCode != "COMM_ROLE_MISMATCH" {
		t.Fatalf("stranger = %#v err=%v", stranger, err)
	}
	if _, err := service.CancelOrder(instruction.OrderID, "CLIENT_CANCEL"); err != nil {
		t.Fatal(err)
	}
	_, closed, err := service.Fulfillment(evidence.BookingID, intent.ClientIdentityID)
	if err != nil || closed.ReasonCode != "COMM_BOOKING_NOT_JOINABLE" {
		t.Fatalf("cancelled join = %#v err=%v", closed, err)
	}
}
