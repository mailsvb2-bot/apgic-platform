package runtimepostgres

import (
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/ledger"
)

func TestBookingLedgerOutboxEventIsDeterministicAndScoped(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	booked := &booking.Booking{
		ID:    "11111111-1111-4111-8111-111111111111",
		State: booking.StateConfirmed,
	}
	entry := ledger.Entry{
		ID:                  "22222222-2222-4222-8222-222222222222",
		EconomicEventRef:    "33333333-3333-4333-8333-333333333333",
		ProviderEvidenceRef: "provider/event-1",
		CorrelationID:       "checkout/correlation-1",
		OccurredAt:          now,
	}

	first, err := bookingLedgerOutboxEvent(booked, entry)
	if err != nil {
		t.Fatal(err)
	}
	second, err := bookingLedgerOutboxEvent(booked, entry)
	if err != nil {
		t.Fatal(err)
	}
	if first.EventID != second.EventID ||
		first.IdempotencyKey != second.IdempotencyKey ||
		first.EventType != "booking.ledger_committed" ||
		first.AggregateRef != "booking/"+booked.ID ||
		first.CausationID != entry.ProviderEvidenceRef {
		t.Fatalf("unexpected outbox event: %#v", first)
	}
	if !sameJSON(first.PayloadJSON, second.PayloadJSON) {
		t.Fatal("same economic effect produced different outbox payload")
	}
}

func TestSameJSONIgnoresObjectFormattingAndOrder(t *testing.T) {
	left := []byte(`{"a":1,"b":{"x":2}}`)
	right := []byte(`{ "b": { "x": 2 }, "a": 1 }`)
	if !sameJSON(left, right) {
		t.Fatal("semantic JSON equality rejected equivalent payloads")
	}
	if sameJSON(left, []byte(`{"a":1,"b":{"x":3}}`)) {
		t.Fatal("semantic JSON equality accepted changed payload")
	}
}
