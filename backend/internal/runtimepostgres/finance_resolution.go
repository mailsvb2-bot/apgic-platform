package runtimepostgres

// R4 reconciliation discrepancy resolution. A discrepancy is never mutated:
// resolution is a typed, append-only fact bound to an explicitly granted
// finance operator, step-up session evidence and an authorization audit
// record. The database guard (migration 000040) re-checks every invariant.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/finance"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

var (
	ErrFinanceResolutionInvalid          = errors.New("finance discrepancy resolution is invalid")
	ErrFinanceOperatorNotGranted         = errors.New("finance operator grant is missing or revoked")
	ErrFinanceDiscrepancyAlreadyResolved = errors.New("finance discrepancy is already resolved")
	ErrFinanceResolutionConflict         = errors.New("finance resolution idempotency key reused with different content")
)

type (
	FinanceDiscrepancyResolutionRequest = finance.DiscrepancyResolutionRequest
	FinanceDiscrepancyResolution        = finance.DiscrepancyResolution
	FinanceOpenDiscrepancy              = finance.OpenDiscrepancy
)

// ActiveFinanceOperatorGrant returns the active grant id for the capability.
func (c *Checker) ActiveFinanceOperatorGrant(ctx context.Context, identityID string, at time.Time) (string, bool, error) {
	if c == nil || c.db == nil {
		return "", false, errors.New("postgres checker is not initialized")
	}
	if !financeUUIDPattern.MatchString(identityID) {
		return "", false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, financeTimeout)
	defer cancel()
	return activeFinanceOperatorGrant(ctx, c.db, identityID, at)
}

type financeQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func activeFinanceOperatorGrant(ctx context.Context, q financeQueryer, identityID string, at time.Time) (string, bool, error) {
	var grantID string
	err := q.QueryRowContext(ctx, `
SELECT g.id::text FROM finance_operator_grants g
 WHERE g.identity_id = $1::uuid AND g.capability = $2 AND g.granted_at <= $3
   AND NOT EXISTS (SELECT 1 FROM finance_operator_grant_revocations r WHERE r.grant_id = g.id AND r.revoked_at <= $3)
 ORDER BY g.granted_at DESC, g.id LIMIT 1`, identityID, finance.CapabilityDiscrepancyResolve, at).Scan(&grantID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read finance operator grant: %w", err)
	}
	return grantID, true, nil
}

// ListOpenFinanceDiscrepancies is the finance-operations work queue.
func (c *Checker) ListOpenFinanceDiscrepancies(ctx context.Context, limit int) ([]FinanceOpenDiscrepancy, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("postgres checker is not initialized")
	}
	ctx, cancel := context.WithTimeout(ctx, financeTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	return listOpenFinanceDiscrepanciesTx(ctx, tx, limit)
}

func listOpenFinanceDiscrepanciesTx(ctx context.Context, tx *sql.Tx, limit int) ([]FinanceOpenDiscrepancy, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := tx.QueryContext(ctx, `
SELECT d.id::text, d.kind, r.settlement_instruction_id::text, run.provider_instance_id::text,
       r.expected_fee_minor, r.provider_fee_minor, r.expected_gross_minor, r.provider_gross_minor,
       r.currency, array_to_string(d.evidence_refs, chr(31)), d.created_at
  FROM finance_reconciliation_discrepancies d
  JOIN finance_reconciliation_results r ON r.id = d.result_id
  JOIN finance_reconciliation_runs run ON run.id = r.run_id
 WHERE NOT EXISTS (SELECT 1 FROM finance_discrepancy_resolutions x WHERE x.discrepancy_id = d.id)
 ORDER BY d.created_at, d.id
 LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list open finance discrepancies: %w", err)
	}
	defer rows.Close()
	out := []FinanceOpenDiscrepancy{}
	for rows.Next() {
		var item FinanceOpenDiscrepancy
		var refs string
		if err := rows.Scan(&item.DiscrepancyID, &item.Kind, &item.SettlementInstructionID, &item.ProviderInstanceID,
			&item.ExpectedFeeMinor, &item.ProviderFeeMinor, &item.ExpectedGrossMinor, &item.ProviderGrossMinor,
			&item.Currency, &refs, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.EvidenceRefs = strings.Split(refs, "\x1f")
		out = append(out, item)
	}
	return out, rows.Err()
}

// ResolveFinanceDiscrepancy appends exactly one typed resolution. Replaying the
// same idempotency key with the same content returns the original record.
func (c *Checker) ResolveFinanceDiscrepancy(ctx context.Context, req FinanceDiscrepancyResolutionRequest) (FinanceDiscrepancyResolution, error) {
	if c == nil || c.db == nil {
		return FinanceDiscrepancyResolution{}, errors.New("postgres checker is not initialized")
	}
	ctx, cancel := context.WithTimeout(ctx, financeTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return FinanceDiscrepancyResolution{}, err
	}
	defer tx.Rollback()
	out, err := resolveFinanceDiscrepancyTx(ctx, tx, req)
	if err != nil {
		return FinanceDiscrepancyResolution{}, err
	}
	return out, tx.Commit()
}

func validateResolutionRequest(req FinanceDiscrepancyResolutionRequest) error {
	if !financeUUIDPattern.MatchString(req.DiscrepancyID) || !financeUUIDPattern.MatchString(req.OperatorID) ||
		!finance.ValidResolutionKind(req.Kind) || strings.TrimSpace(req.IdempotencyKey) == "" ||
		len(strings.TrimSpace(req.Reason)) < 10 || len(req.EvidenceRefs) == 0 ||
		strings.TrimSpace(req.StepUpSessionRef) == "" || strings.TrimSpace(req.AuditRef) == "" || req.ResolvedAt.IsZero() {
		return ErrFinanceResolutionInvalid
	}
	for _, ref := range req.EvidenceRefs {
		if strings.TrimSpace(ref) == "" {
			return ErrFinanceResolutionInvalid
		}
	}
	needEvidence := req.Kind == finance.ResolutionProviderCorrectionConfirmed
	if needEvidence != (req.ProviderEvidenceID != "") || (needEvidence && !financeUUIDPattern.MatchString(req.ProviderEvidenceID)) {
		return ErrFinanceResolutionInvalid
	}
	needResult := req.Kind == finance.ResolutionSupersededByMatched
	if needResult != (req.SupersedingResultID != "") || (needResult && !financeUUIDPattern.MatchString(req.SupersedingResultID)) {
		return ErrFinanceResolutionInvalid
	}
	return nil
}

func resolveFinanceDiscrepancyTx(ctx context.Context, tx *sql.Tx, req FinanceDiscrepancyResolutionRequest) (FinanceDiscrepancyResolution, error) {
	var out FinanceDiscrepancyResolution
	if err := validateResolutionRequest(req); err != nil {
		return out, err
	}
	var instructionID string
	err := tx.QueryRowContext(ctx, `
SELECT r.settlement_instruction_id::text
  FROM finance_reconciliation_discrepancies d
  JOIN finance_reconciliation_results r ON r.id = d.result_id
 WHERE d.id = $1::uuid
   FOR SHARE OF d`, req.DiscrepancyID).Scan(&instructionID)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrFinanceNotFound
	}
	if err != nil {
		return out, fmt.Errorf("lock finance discrepancy: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('finance-resolution:' || $1, 0))`, req.DiscrepancyID); err != nil {
		return out, err
	}

	existing, found, err := readResolutionTx(ctx, tx, `x.discrepancy_id = $1::uuid`, req.DiscrepancyID)
	if err != nil {
		return out, err
	}
	if found {
		var key string
		if err := tx.QueryRowContext(ctx, `SELECT idempotency_key FROM finance_discrepancy_resolutions WHERE id=$1::uuid`, existing.ResolutionID).Scan(&key); err != nil {
			return out, err
		}
		if key != req.IdempotencyKey {
			return out, ErrFinanceDiscrepancyAlreadyResolved
		}
		if existing.Kind != req.Kind || existing.ResolvedBy != req.OperatorID ||
			existing.ProviderEvidenceID != req.ProviderEvidenceID || existing.SupersedingResultID != req.SupersedingResultID {
			return out, ErrFinanceResolutionConflict
		}
		existing.Replayed = true
		return existing, nil
	}
	if _, found, err := readResolutionTx(ctx, tx, `x.idempotency_key = $1`, req.IdempotencyKey); err != nil {
		return out, err
	} else if found {
		return out, ErrFinanceResolutionConflict
	}

	grantID, ok, err := activeFinanceOperatorGrant(ctx, tx, req.OperatorID, req.ResolvedAt)
	if err != nil {
		return out, err
	}
	if !ok || (req.OperatorGrantID != "" && req.OperatorGrantID != grantID) {
		return out, ErrFinanceOperatorNotGranted
	}

	resolutionID, err := persistentid.FromRef("finance-discrepancy-resolution", req.DiscrepancyID)
	if err != nil {
		return out, err
	}
	nullable := func(v string) any {
		if v == "" {
			return nil
		}
		return v
	}
	if _, err := tx.ExecContext(ctx, "SAVEPOINT finance_resolution"); err != nil {
		return out, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO finance_discrepancy_resolutions (id, idempotency_key, discrepancy_id, kind, provider_evidence_id,
  superseding_result_id, resolved_by, operator_grant_id, reason, evidence_refs, step_up_session_ref, audit_ref, resolved_at)
VALUES ($1::uuid,$2,$3::uuid,$4,$5::uuid,$6::uuid,$7::uuid,$8::uuid,$9,$10,$11,$12,$13)`,
		resolutionID, req.IdempotencyKey, req.DiscrepancyID, req.Kind, nullable(req.ProviderEvidenceID),
		nullable(req.SupersedingResultID), req.OperatorID, grantID, strings.TrimSpace(req.Reason),
		"{"+strings.Join(quoteArray(req.EvidenceRefs), ",")+"}", req.StepUpSessionRef, req.AuditRef, req.ResolvedAt); err != nil {
		_, _ = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT finance_resolution")
		return out, fmt.Errorf("%w: %v", ErrFinanceResolutionInvalid, err)
	}
	if err := ensureOutboxEventTx(ctx, tx, financeOutboxEvent(
		"finance-discrepancy-resolved", req.DiscrepancyID, "finance.reconciliation_discrepancy.resolved",
		"settlement/"+instructionID, "finance-discrepancy/"+req.DiscrepancyID, req.ResolvedAt,
		map[string]any{
			"discrepancy_id":            req.DiscrepancyID,
			"resolution_id":             resolutionID,
			"settlement_instruction_id": instructionID,
			"kind":                      req.Kind,
			"audit_ref":                 req.AuditRef,
		})); err != nil {
		return out, err
	}
	out, _, err = readResolutionTx(ctx, tx, `x.id = $1::uuid`, resolutionID)
	return out, err
}

func readResolutionTx(ctx context.Context, tx *sql.Tx, where string, arg any) (FinanceDiscrepancyResolution, bool, error) {
	var out FinanceDiscrepancyResolution
	var evidenceID, resultID sql.NullString
	err := tx.QueryRowContext(ctx, `
SELECT x.id::text, x.discrepancy_id::text, x.kind, x.resolved_by::text, x.resolved_at,
       x.provider_evidence_id::text, x.superseding_result_id::text
  FROM finance_discrepancy_resolutions x WHERE `+where, arg).Scan(
		&out.ResolutionID, &out.DiscrepancyID, &out.Kind, &out.ResolvedBy, &out.ResolvedAt, &evidenceID, &resultID)
	if errors.Is(err, sql.ErrNoRows) {
		return out, false, nil
	}
	if err != nil {
		return out, false, fmt.Errorf("read finance discrepancy resolution: %w", err)
	}
	out.ProviderEvidenceID, out.SupersedingResultID = evidenceID.String, resultID.String
	return out, true, nil
}

func financeDiscrepancyStatusTx(ctx context.Context, tx *sql.Tx, instructionID string) (status, lastKind string, err error) {
	var open, resolved int
	var kind sql.NullString
	err = tx.QueryRowContext(ctx, `
SELECT count(*) FILTER (WHERE x.id IS NULL), count(x.id),
       (array_agg(x.kind ORDER BY x.resolved_at DESC) FILTER (WHERE x.id IS NOT NULL))[1]
  FROM finance_reconciliation_discrepancies d
  JOIN finance_reconciliation_results r ON r.id = d.result_id
  LEFT JOIN finance_discrepancy_resolutions x ON x.discrepancy_id = d.id
 WHERE r.settlement_instruction_id = $1::uuid`, instructionID).Scan(&open, &resolved, &kind)
	if err != nil {
		return "", "", fmt.Errorf("read finance discrepancy status: %w", err)
	}
	switch {
	case open > 0:
		return "OPEN", kind.String, nil
	case resolved > 0:
		return "RESOLVED", kind.String, nil
	}
	return "NONE", "", nil
}
