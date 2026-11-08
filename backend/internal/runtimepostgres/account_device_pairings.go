package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/identity"
)

// CreateAccountDevicePair starts a five-minute browser-to-browser authorization.
// Only hashes are persisted. In particular the claim proof is never stored.
func (c *Checker) CreateAccountDevicePair(id, codeDigest, proofDigest string, expires time.Time) error {
	if c == nil || c.db == nil {
		return errors.New("storage unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// Reap a bounded batch of old one-time proofs. Anonymous requests must
	// not accumulate unlimited expired state in the shared PostgreSQL.
	if _, err := c.db.ExecContext(ctx, `DELETE FROM account_device_pairings WHERE id IN (
	 SELECT id FROM account_device_pairings WHERE expires_at < now()-interval '1 hour'
	 ORDER BY expires_at LIMIT 32)`); err != nil {
		return err
	}
	_, err := c.db.ExecContext(ctx, `INSERT INTO account_device_pairings(id,code_digest,proof_digest,expires_at)
 VALUES($1::uuid,$2,$3,$4)`, id, codeDigest, proofDigest, expires)
	return err
}

// Approval only comes from an already trusted first-party identity session.
func (c *Checker) ApproveAccountDevicePair(codeDigest, identity string) (bool, error) {
	if c == nil || c.db == nil {
		return false, errors.New("storage unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := c.db.ExecContext(ctx, `UPDATE account_device_pairings
 SET identity_id=$2::uuid, approved_at=now()
 WHERE code_digest=$1 AND identity_id IS NULL AND consumed_at IS NULL
 AND expires_at > now() AND EXISTS(SELECT 1 FROM identities WHERE id=$2::uuid)`, codeDigest, identity)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

// PostgreSQL's single UPDATE ... RETURNING is an atomic, one-time claim.
// Concurrency, process restarts and two API instances cannot double-claim.
func (c *Checker) ClaimAccountDevicePair(id, proofDigest string) (string, bool, error) {
	if c == nil || c.db == nil {
		return "", false, errors.New("storage unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var identity string
	err := c.db.QueryRowContext(ctx, `UPDATE account_device_pairings
 SET consumed_at=now()
 WHERE id=$1::uuid AND proof_digest=$2 AND identity_id IS NOT NULL
 AND approved_at IS NOT NULL AND consumed_at IS NULL AND expires_at > now()
 RETURNING identity_id::text`, id, proofDigest).Scan(&identity)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return identity, err == nil, err
}

// Only paired (v3) cookies are admitted by this registry.
func (c *Checker) RegisterAccountPairedSession(id, actor, digest string, expires time.Time) error {
	if c == nil || c.db == nil {
		return errors.New("storage unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := c.db.ExecContext(ctx, `INSERT INTO account_paired_sessions(id,identity_id,cookie_digest,expires_at)
 VALUES($1::uuid,$2::uuid,$3,$4)`, id, actor, digest, expires)
	return err
}
func (c *Checker) AccountPairedSessionActive(actor, digest string) (bool, error) {
	if c == nil || c.db == nil {
		return false, errors.New("storage unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var active bool
	err := c.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_paired_sessions
 WHERE identity_id=$1::uuid AND cookie_digest=$2 AND revoked_at IS NULL AND expires_at>now())`, actor, digest).Scan(&active)
	return active, err
}
func (c *Checker) ListAccountPairedSessions(actor string) ([]identity.PairedDevice, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("storage unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rows, err := c.db.QueryContext(ctx, `SELECT id::text,created_at,expires_at FROM account_paired_sessions
 WHERE identity_id=$1::uuid AND revoked_at IS NULL AND expires_at>now()
 ORDER BY created_at DESC LIMIT 50`, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]identity.PairedDevice, 0)
	for rows.Next() {
		var device identity.PairedDevice
		if err = rows.Scan(&device.ID, &device.CreatedAt, &device.ExpiresAt); err != nil {
			return nil, err
		}
		result = append(result, device)
	}
	return result, rows.Err()
}
func (c *Checker) RevokeAccountPairedSession(actor, id string) (bool, error) {
	if c == nil || c.db == nil {
		return false, errors.New("storage unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := c.db.ExecContext(ctx, `UPDATE account_paired_sessions SET revoked_at=now()
 WHERE identity_id=$1::uuid AND id=$2::uuid AND revoked_at IS NULL`, actor, id)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}
