package audit

import (
	"encoding/json"
	"errors"
	"time"
)

var ErrInvalidRecord = errors.New("audit actor, action, scope, reason and policy version are required")

type Record struct {
	ID            string
	ActorID       string
	Action        string
	Scope         string
	ResourceRef   string
	OldState      json.RawMessage
	NewState      json.RawMessage
	Reason        string
	PolicyVersion string
	OccurredAt    time.Time
	CorrelationID string
}

func New(record Record) (Record, error) {
	if record.ID == "" || record.ActorID == "" || record.Action == "" || record.Scope == "" ||
		record.Reason == "" || record.PolicyVersion == "" {
		return Record{}, ErrInvalidRecord
	}
	if record.OccurredAt.IsZero() {
		record.OccurredAt = time.Now().UTC()
	}
	return record, nil
}

type Appender interface {
	Append(Record) error
}
