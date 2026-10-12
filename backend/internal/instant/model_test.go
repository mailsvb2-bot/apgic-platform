package instant

import "testing"

func TestNoDispatchStateEverGrantsRoomWithoutPaidBooking(t *testing.T) {
	for _, state := range []string{DispatchUnfilled, DispatchOffered, DispatchAccepted, DispatchDeclined, DispatchExpired} {
		d := Dispatch{State: state, RoomReady: true}
		d.Normalize()
		if d.RoomReady {
			t.Fatalf("%s granted an unpaid room", state)
		}
		if state == DispatchAccepted && d.NextAction != ActionPaymentRequired {
			t.Fatalf("accepted dispatch must require payment")
		}
	}
}
func TestUUIDValidation(t *testing.T) {
	for _, value := range []string{"", "not-id", "' OR TRUE --", "0000-0000", "00a19b7e-1180-4a91-86c2-2e4d9a2f9f4d "} {
		if ValidID(value) {
			t.Fatalf("invalid accepted %q", value)
		}
	}
	if !ValidID("00a19b7e-1180-4a91-86c2-2e4d9a2f9f4d") {
		t.Fatal("valid uuid rejected")
	}
}
