package events

import (
	"errors"
	"strings"
	"time"
)

var ErrInvalidOutboxEvent = errors.New("invalid outbox event")

type OutboxEvent struct {
	ID             string
	AggregateType  string
	AggregateID    string
	EventType      string
	IdempotencyKey string
	Payload        []byte
	OccurredAt     time.Time
}

func NewOutboxEvent(id, aggregateType, aggregateID, eventType, idempotencyKey string, payload []byte, occurredAt time.Time) (OutboxEvent, error) {
	event := OutboxEvent{
		ID: id, AggregateType: aggregateType, AggregateID: aggregateID,
		EventType: eventType, IdempotencyKey: idempotencyKey,
		Payload: append([]byte(nil), payload...), OccurredAt: occurredAt,
	}
	if strings.TrimSpace(event.ID) == "" ||
		strings.TrimSpace(event.AggregateType) == "" ||
		strings.TrimSpace(event.AggregateID) == "" ||
		strings.TrimSpace(event.EventType) == "" ||
		strings.TrimSpace(event.IdempotencyKey) == "" ||
		event.OccurredAt.IsZero() {
		return OutboxEvent{}, ErrInvalidOutboxEvent
	}
	return event, nil
}
