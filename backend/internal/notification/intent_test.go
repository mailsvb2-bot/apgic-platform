package notification

import (
	"testing"
	"time"
)

func TestTransactionalIntentDoesNotDependOnGrowthOptIn(t *testing.T) {
	intent, err := NewTransactional(Intent{
		ID: "intent-1", BookingID: "booking-1", Purpose: "BOOKING_CONFIRMATION",
		IdempotencyKey: "booking-1:confirmation", DataClass: "SENSITIVE",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if intent.RequiresGrowthOptIn() {
		t.Fatal("transactional notification must not depend on growth opt-in")
	}
}

func TestDeliveryKeyKeepsOneBusinessIntentAcrossChannels(t *testing.T) {
	intent := Intent{ID: "intent-1"}
	push, err := DeliveryIdempotencyKey(intent, ChannelPush, "device-1")
	if err != nil {
		t.Fatal(err)
	}
	email, err := DeliveryIdempotencyKey(intent, ChannelEmail, "email-1")
	if err != nil {
		t.Fatal(err)
	}
	if push == email {
		t.Fatal("transport delivery keys must be channel scoped")
	}
	if intent.ID != "intent-1" {
		t.Fatal("transport must not replace canonical notification intent")
	}
}
