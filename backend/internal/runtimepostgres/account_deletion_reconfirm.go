package runtimepostgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/privacy"
)

// ReconfirmAccountDeletion binds a verified, fresh step-up assertion to the
// existing canonical deletion request. It does not erase data or make claims
// about provider operations.
func (c *Checker) ReconfirmAccountDeletion(ctx context.Context, identity, sessionRef, method string, verifiedAt time.Time) (privacy.AccountDeletionReceipt, error) {
	var out privacy.AccountDeletionReceipt
	if c == nil || c.db == nil {
		return out, errors.New("postgres unavailable")
	}
	if method != "WEBAUTHN" && method != "TOTP" {
		return out, errors.New("unsupported reauthentication method")
	}
	if identity == "" || sessionRef == "" || verifiedAt.IsZero() {
		return out, errors.New("missing reconfirmation evidence")
	}
	ctx, cancel := context.WithTimeout(ctx, clientPersonalTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", identity); err != nil {
		return out, err
	}
	err = tx.QueryRowContext(ctx, `SELECT id::text,state,source_surface,created_at,updated_at
		FROM delete_account_requests WHERE identity_id=$1::uuid
		ORDER BY created_at DESC,id DESC LIMIT 1 FOR UPDATE`, identity).
		Scan(&out.ID, &out.State, &out.Source, &out.CreatedAt, &out.UpdatedAt)
	if err != nil {
		return privacy.AccountDeletionReceipt{}, err
	}
	if out.State == "IDENTITY_RECONFIRMED" {
		out.Idempotent = true
	} else if out.State != "REQUESTED" {
		return privacy.AccountDeletionReceipt{}, errors.New("request cannot be reconfirmed in current state")
	} else {
		digest := sha256.Sum256([]byte(sessionRef))
		_, err = tx.ExecContext(ctx, `INSERT INTO client_deletion_reconfirmations
			(request_id,identity_id,method,session_ref_hash,verified_at)
			VALUES($1::uuid,$2::uuid,$3,$4,$5)`, out.ID, identity, method, hex.EncodeToString(digest[:]), verifiedAt.UTC())
		if err != nil {
			return privacy.AccountDeletionReceipt{}, fmt.Errorf("persist identity confirmation: %w", err)
		}
		err = tx.QueryRowContext(ctx, `UPDATE delete_account_requests
			SET state='IDENTITY_RECONFIRMED',updated_at=now()
			WHERE id=$1::uuid AND identity_id=$2::uuid AND state='REQUESTED'
			RETURNING state,updated_at`, out.ID, identity).Scan(&out.State, &out.UpdatedAt)
		if err != nil {
			return privacy.AccountDeletionReceipt{}, fmt.Errorf("transition deletion: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return privacy.AccountDeletionReceipt{}, err
	}
	out.Notice = "Личность повторно подтверждена. Удаление ещё не выполнено; ожидается утверждённая политика хранения и доказанное стирание."
	return out, nil
}
