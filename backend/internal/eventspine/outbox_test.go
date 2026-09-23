package eventspine

import (
	"testing"
	"time"
)

func validEvent() EventEnvelope {
	return EventEnvelope{
		EventID:        "evt-1",
		IdempotencyKey: "identity/person-1:role_added:1",
		EventType:      "identity.role_added",
		SchemaVersion:  "1",
		AggregateRef:   "identity/person-1",
		OccurredAt:     time.Now().UTC(),
		Producer:       "identity",
		CorrelationID:  "corr-1",
		PayloadJSON:    []byte(`{"role":"SPECIALIST"}`),
	}
}

func TestOutboxDeliveryMarkIsIdempotent(t *testing.T) {
	record, err := NewRecord(validEvent())
	if err != nil {
		t.Fatal(err)
	}
	first := time.Now().UTC()
	if !record.MarkDelivered(first) {
		t.Fatal("first delivery completion must be recorded")
	}
	second := first.Add(time.Minute)
	if record.MarkDelivered(second) {
		t.Fatal("duplicate delivery completion must not rewrite evidence")
	}
	if record.DeliveredAt == nil || !record.DeliveredAt.Equal(first) {
		t.Fatal("duplicate delivery completion rewrote first delivery evidence")
	}
}

func TestOutboxRejectsDomainRecordsThatDatabaseCannotPersist(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*EventEnvelope)
	}{
		{"idempotency", func(event *EventEnvelope) { event.IdempotencyKey = "" }},
		{"occurred_at", func(event *EventEnvelope) { event.OccurredAt = time.Time{} }},
		{"producer", func(event *EventEnvelope) { event.Producer = "" }},
		{"payload", func(event *EventEnvelope) { event.PayloadJSON = nil }},
		{"invalid_json", func(event *EventEnvelope) { event.PayloadJSON = []byte("{") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event := validEvent()
			tc.mutate(&event)
			if _, err := NewRecord(event); err != ErrInvalidEvent {
				t.Fatalf("expected ErrInvalidEvent, got %v", err)
			}
		})
	}
}

func TestZeroDeliveryTimestampIsRejected(t *testing.T) {
	record, err := NewRecord(validEvent())
	if err != nil {
		t.Fatal(err)
	}
	if record.MarkDelivered(time.Time{}) {
		t.Fatal("zero delivery timestamp must not create terminal evidence")
	}
	if record.Status != Pending || record.DeliveredAt != nil {
		t.Fatal("invalid completion mutated outbox record")
	}
}
