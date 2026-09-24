package calendar

import (
	"errors"
	"strings"
	"time"
)

type State string

const (
	StatePending         State = "PENDING"
	StateSynced          State = "SYNCED"
	StateConflict        State = "CONFLICT"
	StateFailedRetryable State = "FAILED_RETRYABLE"
)

var ErrInvalidSync = errors.New("invalid calendar sync")

type Job struct {
	ID                 string
	BookingID          string
	BookingVersion     int64
	ProviderInstanceID string
	TargetRef          string
	IdempotencyKey     string
	State              State
	ProviderEventRef   string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func NewJob(job Job) (Job, error) {
	if strings.TrimSpace(job.ID) == "" ||
		strings.TrimSpace(job.BookingID) == "" ||
		job.BookingVersion <= 0 ||
		strings.TrimSpace(job.ProviderInstanceID) == "" ||
		strings.TrimSpace(job.TargetRef) == "" ||
		strings.TrimSpace(job.IdempotencyKey) == "" ||
		job.CreatedAt.IsZero() {
		return Job{}, ErrInvalidSync
	}
	job.State = StatePending
	job.UpdatedAt = job.CreatedAt
	job.ProviderEventRef = ""
	return job, nil
}

func (j *Job) ApplyProviderResult(state State, providerEventRef string, now time.Time) error {
	if now.IsZero() || now.Before(j.UpdatedAt) {
		return ErrInvalidSync
	}
	switch state {
	case StateSynced:
		if strings.TrimSpace(providerEventRef) == "" {
			return ErrInvalidSync
		}
		j.ProviderEventRef = providerEventRef
	case StateConflict, StateFailedRetryable:
	default:
		return ErrInvalidSync
	}
	j.State = state
	j.UpdatedAt = now
	return nil
}
