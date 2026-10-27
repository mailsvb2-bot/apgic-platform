package runtimepostgres

import (
	"context"
	"database/sql"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"time"

)

var ErrDeletionFactorInvalid = errors.New("account deletion possession factor is invalid or unavailable")
var ErrDeletionFactorExists = errors.New("account deletion factor already enrolled")
var ErrDeletionFactorCoolingDown = errors.New("account deletion factor not yet eligible for confirmation")

type DeletionFactorStatus struct {
	Enrolled bool      `json:"enrolled"`
	ReadyAt  time.Time `json:"ready_at,omitempty"`
}

func deletionFactorEncrypt(key [32]byte, identity string, secret []byte) ([]byte, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, secret, []byte("apgic-client-deletion-mfa-v1:"+identity)), nil
}

func deletionFactorDecrypt(key [32]byte, identity string, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < gcm.NonceSize()+gcm.Overhead() {
		return nil, ErrDeletionFactorInvalid
	}
	return gcm.Open(nil, ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():], []byte("apgic-client-deletion-mfa-v1:"+identity))
}

// EnrollClientDeletionFactor returns the secret exactly once, and only for an
// existing canonical deletion request belonging to this signed identity.
// A compromised session cannot use its new factor until a 24-hour cooldown.
func (c *Checker) EnrollClientDeletionFactor(ctx context.Context, identity string, key [32]byte, at time.Time) (string, DeletionFactorStatus, error) {
	var zero [32]byte
	if c == nil || c.db == nil || key == zero || !financeUUIDPattern.MatchString(identity) {
		return "", DeletionFactorStatus{}, ErrDeletionFactorInvalid
	}
	secret := make([]byte, 20)
	if _, err := rand.Read(secret); err != nil {
		return "", DeletionFactorStatus{}, err
	}
	encrypted, err := deletionFactorEncrypt(key, identity, secret)
	if err != nil {
		return "", DeletionFactorStatus{}, err
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return "", DeletionFactorStatus{}, err
	}
	defer tx.Rollback()
	var eligible bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM delete_account_requests
		WHERE identity_id=$1::uuid AND state='REQUESTED')`, identity).Scan(&eligible)
	if err != nil || !eligible {
		return "", DeletionFactorStatus{}, ErrDeletionFactorInvalid
	}
	ready := at.UTC().Add(24 * time.Hour)
	result, err := tx.ExecContext(ctx, `INSERT INTO client_deletion_mfa_factors
		(identity_id,encrypted_secret,enrolled_at,ready_at)
		VALUES($1::uuid,$2,$3,$4) ON CONFLICT(identity_id) DO NOTHING`,
		identity, encrypted, at.UTC(), ready)
	if err != nil {
		return "", DeletionFactorStatus{}, err
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return "", DeletionFactorStatus{}, ErrDeletionFactorExists
	}
	if err = tx.Commit(); err != nil {
		return "", DeletionFactorStatus{}, err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret),
		DeletionFactorStatus{Enrolled: true, ReadyAt: ready}, nil
}

func (c *Checker) ClientDeletionFactorStatus(ctx context.Context, identity string) (DeletionFactorStatus, error) {
	var out DeletionFactorStatus
	if c == nil || c.db == nil || !financeUUIDPattern.MatchString(identity) {
		return out, ErrDeletionFactorInvalid
	}
	err := c.db.QueryRowContext(ctx, `SELECT ready_at FROM client_deletion_mfa_factors
		WHERE identity_id=$1::uuid AND revoked_at IS NULL`, identity).Scan(&out.ReadyAt)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Enrolled = true
	return out, nil
}

// VerifyClientDeletionFactor is serialised by owner row, bounded by lockout
// and TOTP last-step monotonicity, and scoped to an existing request.
func (c *Checker) VerifyClientDeletionFactor(ctx context.Context, identity, sessionRef, code string, key [32]byte, at time.Time) (bool, error) {
	var zero [32]byte
	if c == nil || c.db == nil || key == zero ||
		!financeUUIDPattern.MatchString(identity) || len(sessionRef) < 20 || len(code) != 6 {
		return false, ErrDeletionFactorInvalid
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var ciphertext []byte
	var ready time.Time
	var lastStep int64
	var failures int
	var locked *time.Time
	err = tx.QueryRowContext(ctx, `SELECT encrypted_secret,ready_at,last_accepted_step,failure_count,locked_until
		FROM client_deletion_mfa_factors
		WHERE identity_id=$1::uuid AND revoked_at IS NULL FOR UPDATE`, identity).
		Scan(&ciphertext, &ready, &lastStep, &failures, &locked)
	if err != nil {
		return false, ErrDeletionFactorInvalid
	}
	if at.Before(ready) {
		return false, ErrDeletionFactorCoolingDown
	}
	if locked != nil && locked.After(at) {
		return false, ErrDeletionFactorInvalid
	}
	if locked != nil && !locked.After(at) {
		failures = 0
	}
	var active bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM delete_account_requests
		WHERE identity_id=$1::uuid AND state='REQUESTED')`, identity).Scan(&active); err != nil || !active {
		return false, ErrDeletionFactorInvalid
	}
	secret, err := deletionFactorDecrypt(key, identity, ciphertext)
	if err != nil {
		return false, ErrDeletionFactorInvalid
	}
	step := financeMFAMatch(secret, code, at, lastStep)
	if step < 0 {
		failures++
		var blocked any
		if failures >= 5 {
			failures = 5
			blocked = at.Add(15 * time.Minute).UTC()
		}
		_, err = tx.ExecContext(ctx, `UPDATE client_deletion_mfa_factors
			SET failure_count=$2,locked_until=$3 WHERE identity_id=$1::uuid`,
			identity, failures, blocked)
		if err != nil {
			return false, err
		}
		if err = tx.Commit(); err != nil {
			return false, err
		}
		return false, ErrDeletionFactorInvalid
	}
	_, err = tx.ExecContext(ctx, `UPDATE client_deletion_mfa_factors
		SET last_accepted_step=$2,failure_count=0,locked_until=NULL
		WHERE identity_id=$1::uuid`, identity, step)
	if err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// DeletionFactorActiveAt also revokes previously minted step-up assertions
// when the underlying factor is retired or the owner has been deleted.
func (c *Checker) DeletionFactorActiveAt(ctx context.Context, identity string, issuedAt time.Time) (bool, error) {
	if c == nil || c.db == nil {
		return false, ErrDeletionFactorInvalid
	}
	var active bool
	err := c.db.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM client_deletion_mfa_factors f
		WHERE f.identity_id=$1::uuid AND f.revoked_at IS NULL
		  AND f.ready_at <= $2
		  AND NOT EXISTS(SELECT 1 FROM deletion_revoked_identities x WHERE x.identity_id=f.identity_id))`,
		identity, issuedAt.UTC()).Scan(&active)
	return active, err
}
