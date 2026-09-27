package demand

import "testing"

func TestBookingIDForHoldIsStableAndDistinct(t *testing.T) {
	holdID, err := newJourneyID()
	if err != nil {
		t.Fatal(err)
	}
	first, err := bookingIDForHold(holdID)
	if err != nil {
		t.Fatal(err)
	}
	again, err := bookingIDForHold(holdID)
	if err != nil {
		t.Fatal(err)
	}
	otherHold, _ := newJourneyID()
	other, err := bookingIDForHold(otherHold)
	if err != nil {
		t.Fatal(err)
	}
	if first != again || first == other {
		t.Fatalf("booking id derivation mismatch first=%s again=%s other=%s", first, again, other)
	}
}
