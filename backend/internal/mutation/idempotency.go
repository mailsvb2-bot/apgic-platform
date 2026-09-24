package mutation

import (
	"errors"
	"strings"
)

type Outcome string

const (
	OutcomeClaimed   Outcome = "CLAIMED"
	OutcomeDuplicate Outcome = "DUPLICATE"
	OutcomeConflict  Outcome = "CONFLICT"
)

var ErrInvalidMutation = errors.New("invalid client mutation")

type Envelope struct {
	IdentityID     string
	Operation      string
	IdempotencyKey string
	RequestDigest  string
}

func (e Envelope) Validate() error {
	if strings.TrimSpace(e.IdentityID) == "" ||
		strings.TrimSpace(e.Operation) == "" ||
		strings.TrimSpace(e.IdempotencyKey) == "" ||
		strings.TrimSpace(e.RequestDigest) == "" {
		return ErrInvalidMutation
	}
	return nil
}

func ClassifyRetry(existing, incoming Envelope) (Outcome, error) {
	if err := existing.Validate(); err != nil {
		return "", err
	}
	if err := incoming.Validate(); err != nil {
		return "", err
	}
	if existing.IdentityID != incoming.IdentityID ||
		existing.Operation != incoming.Operation ||
		existing.IdempotencyKey != incoming.IdempotencyKey {
		return OutcomeClaimed, nil
	}
	if existing.RequestDigest == incoming.RequestDigest {
		return OutcomeDuplicate, nil
	}
	return OutcomeConflict, nil
}
