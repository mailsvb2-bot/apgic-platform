package connector

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

var ErrInvalidRequest = errors.New("connector request requires idempotency key, subject id and valid optional JSON payload")

type Request struct {
	IdempotencyKey string
	SubjectID      string
	Payload        json.RawMessage
}

func (r Request) Validate() error {
	if strings.TrimSpace(r.IdempotencyKey) == "" || strings.TrimSpace(r.SubjectID) == "" {
		return ErrInvalidRequest
	}
	if len(r.Payload) > 0 && !json.Valid(r.Payload) {
		return ErrInvalidRequest
	}
	return nil
}

type Outcome string

const (
	OutcomeSuccess          Outcome = "SUCCESS"
	OutcomeRetryableFailure Outcome = "RETRYABLE_FAILURE"
	OutcomeAmbiguous        Outcome = "AMBIGUOUS"
	OutcomeTerminalFailure  Outcome = "TERMINAL_FAILURE"
)

func (o Outcome) Valid() bool {
	switch o {
	case OutcomeSuccess, OutcomeRetryableFailure, OutcomeAmbiguous, OutcomeTerminalFailure:
		return true
	default:
		return false
	}
}

type Result struct {
	ProviderReference string
	Outcome           Outcome
	RawEvidence       json.RawMessage
}

func (r Result) Validate() error {
	if !r.Outcome.Valid() {
		return errors.New("connector provider returned invalid outcome")
	}
	if len(r.RawEvidence) > 0 && !json.Valid(r.RawEvidence) {
		return errors.New("connector provider returned invalid evidence JSON")
	}
	return nil
}

func ShouldRetry(outcome Outcome, completedAttempts, maxAttempts uint32) bool {
	return outcome == OutcomeRetryableFailure &&
		maxAttempts > 0 &&
		completedAttempts < maxAttempts
}

type Provider interface {
	Kind() string
	Capabilities() []CapabilityClass
	Execute(context.Context, CapabilityClass, Request) (Result, error)
}
