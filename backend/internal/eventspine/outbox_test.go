package eventspine

import (
	"testing"
	"time"
)

func TestOutboxDeliveryMarkIsIdempotent(t *testing.T) {
	rec, err := NewRecord(EventEnvelope{
		EventID: "evt-1", EventType: "identity.role_added", SchemaVersion: "1",
		AggregateRef: "identity/person-1", CorrelationID: "corr-1",
		OccurredAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	first := time.Now().UTC()
	rec.MarkDelivered(first)
	second := first.Add(time.Minute)
	rec.MarkDelivered(second)
	if rec.DeliveredAt == nil || !rec.DeliveredAt.Equal(first) {
		t.Fatal("duplicate delivery completion must not rewrite first delivery evidence")
	}
}
