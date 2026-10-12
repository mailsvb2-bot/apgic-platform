package connector

import (
	"errors"
	"time"
)

var (
	ErrConsultationNotReady        = errors.New("consultation is not in readiness window")
	ErrConsultationOutcomeNotReady = errors.New("consultation does not have verified completion")
	ErrConsultationOutcomeConflict = errors.New("outcome idempotency key reused with different content")
	ErrConsultationOutcomeInvalid  = errors.New("invalid consultation outcome")
)

// ReadySnapshot is a participant-authenticated booking projection, not provider media evidence.
type ReadySnapshot struct {
	BookingID         string     `json:"booking_id"`
	Role              string     `json:"role"`
	BookingState      string     `json:"booking_state"`
	OpensAt           time.Time  `json:"opens_at"`
	ClosesAt          time.Time  `json:"closes_at"`
	ClientReadyAt     *time.Time `json:"client_ready_at,omitempty"`
	SpecialistReadyAt *time.Time `json:"specialist_ready_at,omitempty"`
}

// OutcomeReport contains deliberately bounded, client-reported signals.
// No transcript, recording, medical diagnosis or private notes are collected.
type OutcomeReport struct {
	BookingID         string    `json:"booking_id"`
	Progress          int       `json:"progress"`
	MatchSatisfaction int       `json:"match_satisfaction"`
	ReportedAt        time.Time `json:"reported_at"`
	Idempotent        bool      `json:"idempotent"`
}
