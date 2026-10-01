package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/mutation"
)

func (c *Checker) Claim(ctx context.Context, mutationID string, envelope mutation.Envelope, now time.Time) (mutation.ClaimResult, error) {
	if c == nil || c.db == nil {
		return mutation.ClaimResult{}, errors.New("postgres checker is not initialized")
	}
	if strings.TrimSpace(mutationID) == "" || now.IsZero() {
		return mutation.ClaimResult{}, mutation.ErrInvalidMutation
	}
	if err := envelope.Validate(); err != nil {
		return mutation.ClaimResult{}, err
	}

	var result mutation.ClaimResult
	var outcome string
	if err := c.db.QueryRowContext(
		ctx,
		`SELECT outcome, mutation_id::text, mutation_state, COALESCE(side_effect_ref, '')
		   FROM apgic_claim_client_mutation($1::uuid, $2::uuid, $3, $4, $5, $6, $7)`,
		mutationID,
		envelope.IdentityID,
		envelope.Operation,
		envelope.IdempotencyKey,
		envelope.CorrelationID,
		envelope.RequestDigest,
		now.UTC(),
	).Scan(&outcome, &result.MutationID, &result.State, &result.SideEffectRef); err != nil {
		return mutation.ClaimResult{}, err
	}
	result.Outcome = mutation.Outcome(outcome)
	if result.Outcome == mutation.OutcomeFailed {
		if err := c.db.QueryRowContext(
			ctx,
			`SELECT COALESCE(failure_code, '') FROM client_mutation_records WHERE id = $1::uuid`,
			result.MutationID,
		).Scan(&result.FailureCode); err != nil {
			return mutation.ClaimResult{}, err
		}
	}
	return result, nil
}

func (c *Checker) Finalize(ctx context.Context, mutationID, sideEffectRef string, now time.Time) (mutation.FinalizeResult, error) {
	if c == nil || c.db == nil {
		return mutation.FinalizeResult{}, errors.New("postgres checker is not initialized")
	}
	if strings.TrimSpace(mutationID) == "" || strings.TrimSpace(sideEffectRef) == "" || now.IsZero() {
		return mutation.FinalizeResult{}, mutation.ErrInvalidMutation
	}
	var result mutation.FinalizeResult
	if err := c.db.QueryRowContext(
		ctx,
		`SELECT applied, reason_code FROM apgic_finalize_client_mutation($1::uuid, $2, $3)`,
		mutationID,
		sideEffectRef,
		now.UTC(),
	).Scan(&result.Changed, &result.ReasonCode); err != nil {
		return mutation.FinalizeResult{}, err
	}
	return result, nil
}

func (c *Checker) Fail(ctx context.Context, mutationID, failureCode string, now time.Time) (mutation.FinalizeResult, error) {
	if c == nil || c.db == nil {
		return mutation.FinalizeResult{}, errors.New("postgres checker is not initialized")
	}
	if strings.TrimSpace(mutationID) == "" || strings.TrimSpace(failureCode) == "" || now.IsZero() {
		return mutation.FinalizeResult{}, mutation.ErrInvalidMutation
	}
	var result mutation.FinalizeResult
	if err := c.db.QueryRowContext(
		ctx,
		`SELECT failed, reason_code FROM apgic_fail_client_mutation($1::uuid, $2, $3)`,
		mutationID,
		failureCode,
		now.UTC(),
	).Scan(&result.Changed, &result.ReasonCode); err != nil {
		return mutation.FinalizeResult{}, err
	}
	return result, nil
}

func (c *Checker) mutationFailureCode(ctx context.Context, mutationID string) (string, error) {
	var code sql.NullString
	if err := c.db.QueryRowContext(
		ctx,
		`SELECT failure_code FROM client_mutation_records WHERE id = $1::uuid`,
		mutationID,
	).Scan(&code); err != nil {
		return "", err
	}
	if !code.Valid {
		return "", nil
	}
	return code.String, nil
}
