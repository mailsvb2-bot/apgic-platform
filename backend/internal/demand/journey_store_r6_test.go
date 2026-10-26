package demand

import (
	"errors"
	"testing"
)

// R2 must translate the canonical PostgreSQL duty exclusion into an ordinary
// user-visible availability conflict, not a storage outage or protocol error.
func TestDutyBusyHoldReasonUsesCanonicalSlotUnavailable(t *testing.T) {
	if err := journeyReasonError("BOOK_SLOT_DUTY_BUSY"); !errors.Is(err, ErrSlotUnavailable) {
		t.Fatalf("R6 occupied specialist must yield canonical slot unavailable, got %v", err)
	}
	if err := journeyReasonError("BOOK_SLOT_UNEXPECTED_DUTY_STATE"); !errors.Is(err, ErrJourneyStoreProtocol) {
		t.Fatalf("unknown booking reason must still fail closed, got %v", err)
	}
}
