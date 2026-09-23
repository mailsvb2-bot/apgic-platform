package eventspine

import (
	"errors"
	"time"
)

type DeliveryStatus string

const (
	Pending   DeliveryStatus = "PENDING"
	Delivered DeliveryStatus = "DELIVERED"
)

var ErrInvalidEvent = errors.New("event id, type, schema version, aggregate ref and correlation id are required")

type EventEnvelope struct {
	EventID          string
	EventType        string
	SchemaVersion    string
	AggregateRef     string
	AggregateVersion uint64
	OccurredAt       time.Time
	ProducedAt       time.Time
	Producer         string
	TenantScope      string
	CorrelationID    string
	CausationID      string
	PayloadJSON      []byte
}

type OutboxRecord struct {
	Event       EventEnvelope
	Status      DeliveryStatus
	Attempts    uint32
	DeliveredAt *time.Time
}

func NewRecord(event EventEnvelope) (OutboxRecord, error) {
	if event.EventID == "" || event.EventType == "" || event.SchemaVersion == "" ||
		event.AggregateRef == "" || event.CorrelationID == "" {
		return OutboxRecord{}, ErrInvalidEvent
	}
	if event.ProducedAt.IsZero() {
		event.ProducedAt = time.Now().UTC()
	}
	return OutboxRecord{Event: event, Status: Pending}, nil
}

func (o *OutboxRecord) MarkDelivered(at time.Time) {
	if o.Status == Delivered {
		return
	}
	o.Status = Delivered
	o.DeliveredAt = &at
}
