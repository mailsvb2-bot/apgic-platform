package mutation

import (
	"context"
	"time"
)

const (
	OutcomeDuplicateApplied Outcome = "DUPLICATE_APPLIED"
	OutcomeFailed           Outcome = "FAILED"
)

const (
	StateClaimed = "CLAIMED"
	StateApplied = "APPLIED"
	StateFailed  = "FAILED"
)

type ClaimResult struct {
	Outcome       Outcome
	MutationID    string
	State         string
	SideEffectRef string
	FailureCode   string
}

type FinalizeResult struct {
	Changed    bool
	ReasonCode string
}

type Store interface {
	Claim(context.Context, string, Envelope, time.Time) (ClaimResult, error)
	Finalize(context.Context, string, string, time.Time) (FinalizeResult, error)
	Fail(context.Context, string, string, time.Time) (FinalizeResult, error)
}
