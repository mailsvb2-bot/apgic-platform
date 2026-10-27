package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/privacy"
)

const deletionIdentityReconfirmedNotice = "Личность подтверждена. Удаление ещё не выполнено: требуется утверждённая политика хранения и подтверждение фактического стирания."

// ReconfirmAccountDeletionRequest commits immutable, verified factor evidence
// against the canonical request. It never advances retention or erasure.
func (c *Checker) ReconfirmAccountDeletionRequest(ctx context.Context, identity, sessionRef string, issuedAt time.Time) (privacy.AccountDeletionReceipt, error) {
	var out privacy.AccountDeletionReceipt
	if c == nil || c.db == nil {
		return out, errors.New("postgres unavailable")
	}
	age := time.Since(issuedAt)
	if strings.TrimSpace(identity) == "" || !strings.HasPrefix(sessionRef, "session:") ||
		len(sessionRef) < len("session:")+32 || issuedAt.IsZero() ||
		age < -time.Minute || age > 10*time.Minute {
		return out, errors.New("invalid independently verified deletion confirmation")
	}
	ctx, cancel := context.WithTimeout(ctx, clientPersonalTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	// Row lock serializes reconfirmation; an old request cannot be superseded.
	err = tx.QueryRowContext(ctx, `SELECT id::text,state,source_surface,created_at,updated_at
		FROM delete_account_requests WHERE identity_id=$1::uuid
		ORDER BY created_at DESC,id DESC LIMIT 1 FOR UPDATE`, identity).
		Scan(&out.ID, &out.State, &out.Source, &out.CreatedAt, &out.UpdatedAt)
	if err != nil {
		return out, fmt.Errorf("read canonical deletion request: %w", err)
	}
	if out.State == "REQUESTED" {
		err = tx.QueryRowContext(ctx, `UPDATE delete_account_requests
			SET state='IDENTITY_RECONFIRMED', reconfirmed_at=$3::timestamptz,
			    reconfirmation_method='VERIFIED_TOTP',
			    reconfirmation_session_ref=$4, updated_at=now()
			WHERE id=$1::uuid AND identity_id=$2::uuid AND state='REQUESTED'
			RETURNING id::text,state,source_surface,created_at,updated_at`,
			out.ID, identity, issuedAt.UTC(), sessionRef).
			Scan(&out.ID, &out.State, &out.Source, &out.CreatedAt, &out.UpdatedAt)
		if err != nil {
			return privacy.AccountDeletionReceipt{}, fmt.Errorf("persist verified confirmation: %w", err)
		}
	} else if out.State == "IDENTITY_RECONFIRMED" {
		out.Idempotent = true
	} else {
		// Later legal/executor states may be observed, but never rewritten
		// by an old proof from a different login.
		out.Idempotent = true
	}
	if err = tx.Commit(); err != nil {
		return privacy.AccountDeletionReceipt{}, err
	}
	verified, err := c.ReadAccountDeletionRequest(ctx, identity)
	if err != nil {
		return privacy.AccountDeletionReceipt{}, err
	}
	verified.Idempotent = out.Idempotent
	return verified, nil
}
