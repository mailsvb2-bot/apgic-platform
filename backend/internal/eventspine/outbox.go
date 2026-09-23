package eventspine

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type DeliveryStatus string

const (
	Pending   DeliveryStatus = "PENDING"
	Delivered DeliveryStatus = "DELIVERED"
)

var ErrInvalidEvent = errors.New("outbox event is incomplete or invalid")

type EventEnvelope struct {
	EventID          string
	IdempotencyKey   string
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
	if strings.TrimSpace(event.EventID) == "" ||
		strings.TrimSpace(event.IdempotencyKey) == "" ||
		strings.TrimSpace(event.EventType) == "" ||
		strings.TrimSpace(event.SchemaVersion) == "" ||
		strings.TrimSpace(event.AggregateRef) == "" ||
		event.OccurredAt.IsZero() ||
		strings.TrimSpace(event.Producer) == "" ||
		strings.TrimSpace(event.CorrelationID) == "" ||
		len(event.PayloadJSON) == 0 ||
		!json.Valid(event.PayloadJSON) {
		return OutboxRecord{}, ErrInvalidEvent
	}
	if event.ProducedAt.IsZero() {
		event.ProducedAt = time.Now().UTC()
	}
	event.PayloadJSON = append([]byte(nil), event.PayloadJSON...)
	return OutboxRecord{Event: event, Status: Pending}, nil
}

func (o *OutboxRecord) MarkDelivered(at time.Time) bool {
	if o.Status == Delivered || at.IsZero() {
		return false
	}
	o.Status = Delivered
	o.DeliveredAt = &at
	return true
}
