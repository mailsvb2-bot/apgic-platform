package instant

// Fulfillment describes only persisted canonical R2 evidence. An R6 dispatch
// cannot mint payment or R3 room readiness; joining remains owned by R3.
type Fulfillment struct {
	DispatchID      string `json:"dispatch_id"`
	DispatchState   string `json:"dispatch_state"`
	BookingID       string `json:"booking_id,omitempty"`
	BookingState    string `json:"booking_state,omitempty"`
	PaymentVerified bool   `json:"payment_verified"`
	NextAction      string `json:"next_action"`
	RoomReady       bool   `json:"room_ready"`
}

const (
	ActionAwaitBooking     = "AWAIT_CANONICAL_R2_BOOKING"
	ActionAwaitProvider    = "AWAIT_VERIFIED_PROVIDER_PAYMENT"
	ActionConsultationGate = "USE_R3_BOOKING_ACCESS_GATE"
)

type FulfillmentReader interface {
	ReadFulfillment(dispatchID, clientIdentityID string) (Fulfillment, error)
}
