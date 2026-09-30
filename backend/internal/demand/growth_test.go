package demand

import (
	"errors"
	"testing"
)

func TestGrowthCannotReceiveRawConsultationWithoutPurposeConsent(t *testing.T) {
	service, bookingID := confirmedSession(t)
	done, err := service.CompleteSession(bookingID, "provider-room-end")
	if err != nil || done.RawContentStored || done.State != "COMPLETED" {
		t.Fatalf("complete = %#v err=%v", done, err)
	}
	if _, err := service.ExportSessionToGrowth(bookingID, false); !errors.Is(err, ErrPurposeConsent) {
		t.Fatalf("export without consent err = %v", err)
	}
	exported, err := service.ExportSessionToGrowth(bookingID, true)
	if err != nil || !exported.Allowed || exported.RawContentIncluded || exported.State != "COMPLETED" {
		t.Fatalf("consented export = %#v err=%v", exported, err)
	}
}

func TestGrowthExportRequiresBookingOwner(t *testing.T) {
	service := NewConformanceService(nil)
	intent, err := service.CreateIntentForIdentity(
		"11111111-1111-4111-8111-111111111111",
		"бессонница",
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmIntent(intent.ID, []string{"sleep"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	slots, err := service.Slots("spec-lebedeva")
	if err != nil || len(slots) == 0 {
		t.Fatalf("slots=%#v err=%v", slots, err)
	}
	hold, err := service.AcquireHold(intent.ID, slots[0].ID, intent.ClientIdentityID)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RequireBookingOwner(hold.BookingID, intent.ClientIdentityID); err != nil {
		t.Fatalf("owner rejected: %v", err)
	}
	if err := service.RequireBookingOwner(
		hold.BookingID,
		"22222222-2222-4222-8222-222222222222",
	); !errors.Is(err, ErrBookingIdentityMismatch) {
		t.Fatalf("cross-subject booking access err=%v", err)
	}
	if err := service.RequireBookingOwner(
		"33333333-3333-4333-8333-333333333333",
		intent.ClientIdentityID,
	); !errors.Is(err, ErrBookingNotFound) {
		t.Fatalf("missing booking ownership err=%v", err)
	}
}
