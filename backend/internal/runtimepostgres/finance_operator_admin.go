package runtimepostgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/finance"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

var (
	ErrFinanceOperatorAlreadyGranted = errors.New("finance operator already has an active grant")
	ErrFinanceOperatorGrantNotFound  = errors.New("finance operator grant not found")
	ErrFinanceOperatorGrantRevoked   = errors.New("finance operator grant already revoked")
)

// GrantFinanceOperator provisions an out-of-band capability using a controlled
// database credential. The immutable grant and its audit record commit atomically.
func (c *Checker) GrantFinanceOperator(ctx context.Context, identityID, approvedBy, approvalRef string, at time.Time) (string, error) {
	identityID, approvedBy, approvalRef = strings.TrimSpace(identityID), strings.TrimSpace(approvedBy), strings.TrimSpace(approvalRef)
	if c == nil || c.db == nil {
		return "", errors.New("postgres checker is not initialized")
	}
	if !financeUUIDPattern.MatchString(identityID) || approvedBy == "" || approvalRef == "" || at.IsZero() {
		return "", finance.ErrResolutionInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, financeTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin finance operator grant: %w", err)
	}
	defer tx.Rollback()

	var activeID string
	err = tx.QueryRowContext(ctx, `SELECT g.id::text
		FROM finance_operator_grants g
		WHERE g.identity_id=$1::uuid AND g.capability=$2
		  AND NOT EXISTS (SELECT 1 FROM finance_operator_grant_revocations r WHERE r.grant_id=g.id)
		ORDER BY g.granted_at DESC, g.id LIMIT 1 FOR UPDATE`,
		identityID, finance.CapabilityDiscrepancyResolve).Scan(&activeID)
	if err == nil {
		return "", ErrFinanceOperatorAlreadyGranted
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("check active finance operator grant: %w", err)
	}
	grantID, err := persistentid.New()
	if err != nil {
		return "", err
	}
	auditID, err := persistentid.New()
	if err != nil {
		return "", err
	}
	state, _ := json.Marshal(map[string]string{"grant_id": grantID, "identity_id": identityID, "capability": finance.CapabilityDiscrepancyResolve, "approval_ref": approvalRef})
	if _, err := tx.ExecContext(ctx, `INSERT INTO finance_operator_grants
		(id, identity_id, capability, granted_by, grant_ref, granted_at)
		VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6)`,
		grantID, identityID, finance.CapabilityDiscrepancyResolve, approvedBy, approvalRef, at.UTC()); err != nil {
		return "", fmt.Errorf("persist finance operator grant: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_records
		(id, actor_id, action, scope, resource_ref, new_state, reason, policy_version, correlation_id, occurred_at)
		VALUES ($1::uuid,$2,$3,$4,$5,$6::jsonb,$7,$8,$9,$10)`,
		auditID, approvedBy, "finance.operator.grant", "apgic/finance-operations",
		"identity/"+identityID, string(state), "approved grant reference: "+approvalRef,
		"finance-operator-grant-v1", "finance-operator-grant/"+grantID, at.UTC()); err != nil {
		return "", fmt.Errorf("audit finance operator grant: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit finance operator grant: %w", err)
	}
	return grantID, nil
}

// RevokeFinanceOperator records an immutable revocation and audit record in one
// transaction. The grant row itself is never updated or deleted.
func (c *Checker) RevokeFinanceOperator(ctx context.Context, grantID, approvedBy, reason string, at time.Time) error {
	grantID, approvedBy, reason = strings.TrimSpace(grantID), strings.TrimSpace(approvedBy), strings.TrimSpace(reason)
	if c == nil || c.db == nil {
		return errors.New("postgres checker is not initialized")
	}
	if !financeUUIDPattern.MatchString(grantID) || approvedBy == "" || len(reason) < 10 || at.IsZero() {
		return finance.ErrResolutionInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, financeTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin finance operator revocation: %w", err)
	}
	defer tx.Rollback()

	var identityID, capability string
	err = tx.QueryRowContext(ctx, `SELECT identity_id::text, capability
		FROM finance_operator_grants WHERE id=$1::uuid FOR UPDATE`, grantID).Scan(&identityID, &capability)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrFinanceOperatorGrantNotFound
	}
	if err != nil {
		return fmt.Errorf("read finance operator grant: %w", err)
	}
	if capability != finance.CapabilityDiscrepancyResolve {
		return finance.ErrResolutionInvalid
	}
	var revoked bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM finance_operator_grant_revocations WHERE grant_id=$1::uuid)`, grantID).Scan(&revoked); err != nil {
		return err
	}
	if revoked {
		return ErrFinanceOperatorGrantRevoked
	}
	revocationID, err := persistentid.New()
	if err != nil {
		return err
	}
	auditID, err := persistentid.New()
	if err != nil {
		return err
	}
	state, _ := json.Marshal(map[string]string{"grant_id": grantID, "identity_id": identityID, "revocation_id": revocationID})
	if _, err := tx.ExecContext(ctx, `INSERT INTO finance_operator_grant_revocations
		(id, grant_id, revoked_by, reason, revoked_at) VALUES ($1::uuid,$2::uuid,$3,$4,$5)`,
		revocationID, grantID, approvedBy, reason, at.UTC()); err != nil {
		return fmt.Errorf("persist finance operator revocation: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_records
		(id, actor_id, action, scope, resource_ref, new_state, reason, policy_version, correlation_id, occurred_at)
		VALUES ($1::uuid,$2,$3,$4,$5,$6::jsonb,$7,$8,$9,$10)`,
		auditID, approvedBy, "finance.operator.revoke", "apgic/finance-operations",
		"finance-operator-grant/"+grantID, string(state), reason,
		"finance-operator-grant-v1", "finance-operator-revoke/"+grantID, at.UTC()); err != nil {
		return fmt.Errorf("audit finance operator revocation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit finance operator revocation: %w", err)
	}
	return nil
}
