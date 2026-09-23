package audit

import (
	"errors"
	"strings"
	"time"
)

var ErrInvalidRecord = errors.New("invalid audit record")

type Record struct {
	ID            string
	ActorID       string
	Scope         string
	Action        string
	Reason        string
	PolicyVersion string
	OldState      []byte
	NewState      []byte
	OccurredAt    time.Time
}

func New(record Record) (Record, error) {
	if strings.TrimSpace(record.ID) == "" ||
		strings.TrimSpace(record.ActorID) == "" ||
		strings.TrimSpace(record.Scope) == "" ||
		strings.TrimSpace(record.Action) == "" ||
		strings.TrimSpace(record.Reason) == "" ||
		strings.TrimSpace(record.PolicyVersion) == "" ||
		record.OccurredAt.IsZero() {
		return Record{}, ErrInvalidRecord
	}
	record.OldState = append([]byte(nil), record.OldState...)
	record.NewState = append([]byte(nil), record.NewState...)
	return record, nil
}
