package demand

import (
	"errors"
	"testing"
	"time"
)

func TestHoldStatusAuthorizesOwnerAndExpiresAtServer(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	service := NewConformanceService(func() time.Time { return now })
	intent, err := service.CreateIntent("Мне сложно уснуть")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ConfirmIntent(intent.ID, []string{"sleep"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	slots, err := service.Slots("spec-lebedeva")
	if err != nil || len(slots) == 0 {
		t.Fatalf("slots: %v %v", slots, err)
	}
	hold, err := service.AcquireHold(intent.ID, slots[0].ID, intent.ClientIdentityID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.HoldStatus(hold.ID, "someone-else"); !errors.Is(err, ErrBookingIdentityMismatch) {
		t.Fatalf("other identity could read hold: %v", err)
	}
	status, err := service.HoldStatus(hold.ID, intent.ClientIdentityID)
	if err != nil || status.BookingState != "HELD" {
		t.Fatalf("owned hold status: %v, %v", status, err)
	}
	status.State = "TAMPERED"
	reread, err := service.HoldStatus(hold.ID, intent.ClientIdentityID)
	if err != nil || reread.State == "TAMPERED" {
		t.Fatalf("status exposed mutable state: %v, %v", reread, err)
	}
	now = hold.ExpiresAt.Add(time.Second)
	expired, err := service.HoldStatus(hold.ID, intent.ClientIdentityID)
	if err != nil || expired.State != "EXPIRED" || expired.BookingState != "EXPIRED" {
		t.Fatalf("authoritative expiry missing: %v, %v", expired, err)
	}
}
