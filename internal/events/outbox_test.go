package events

import (
	"testing"
	"time"
)

func TestOutboxRequiresIdempotencyIdentity(t *testing.T) {
	_, err := NewOutboxEvent(
		"evt-1", "booking", "booking-1", "booking.created", "",
		[]byte(`{"booking_id":"booking-1"}`), time.Now(),
	)
	if err == nil {
		t.Fatal("missing idempotency key must be rejected")
	}
}
