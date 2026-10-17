package runtimepostgres

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

var ErrFinanceMFAInvalid = errors.New("finance MFA proof invalid or unavailable")
var ErrFinanceMFAExists = errors.New("finance MFA already enrolled")

// FinanceMFAKey is a 32-byte AES-GCM key independent from the session cookie
// key. The key must be supplied by the host secret manager, not Git or clients.
type FinanceMFAKey = [32]byte

func financeMFAEncrypt(key FinanceMFAKey, identity string, secret []byte) ([]byte, error) {
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
	return gcm.Seal(nonce, nonce, secret, []byte("apgic-finance-mfa-v1:"+identity)), nil
}
func financeMFADecrypt(key FinanceMFAKey, identity string, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < gcm.NonceSize()+gcm.Overhead() {
		return nil, ErrFinanceMFAInvalid
	}
	return gcm.Open(nil, ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():], []byte("apgic-finance-mfa-v1:"+identity))
}
func financeMFACode(secret []byte, step int64) string {
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	mac := hmac.New(sha1.New, secret)
	_, _ = mac.Write(counter[:])
	sum := mac.Sum(nil)
	off := int(sum[len(sum)-1] & 15)
	number := (uint32(sum[off]&0x7f) << 24) | (uint32(sum[off+1]) << 16) | (uint32(sum[off+2]) << 8) | uint32(sum[off+3])
	return fmt.Sprintf("%06d", number%1000000)
}
func financeMFAMatch(secret []byte, code string, now time.Time, lastStep int64) int64 {
	if len(code) != 6 {
		return -1
	}
	for _, digit := range code {
		if digit < '0' || digit > '9' {
			return -1
		}
	}
	step := now.Unix() / 30
	for _, candidate := range []int64{step, step - 1, step + 1} {
		if candidate < 0 || candidate <= lastStep {
			continue
		}
		if hmac.Equal([]byte(financeMFACode(secret, candidate)), []byte(code)) {
			return candidate
		}
	}
	return -1
}

// ProvisionFinanceMFA requires an out-of-band controlled admin approval and
// an existing operator grant. It never replaces an enrolled factor implicitly.
func (c *Checker) ProvisionFinanceMFA(ctx context.Context, identity, approver, approval string, key FinanceMFAKey, now time.Time) (string, error) {
	if c == nil || c.db == nil || !financeUUIDPattern.MatchString(identity) || strings.TrimSpace(approver) == "" || strings.TrimSpace(approval) == "" {
		return "", ErrFinanceMFAInvalid
	}
	var empty FinanceMFAKey
	if key == empty {
		return "", ErrFinanceMFAInvalid
	}
	secret := make([]byte, 20)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	encrypted, err := financeMFAEncrypt(key, identity, secret)
	if err != nil {
		return "", err
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var granted bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(
  SELECT 1 FROM finance_operator_grants g WHERE g.identity_id=$1::uuid
  AND g.capability='FINANCE_DISCREPANCY_RESOLVE'
  AND NOT EXISTS(SELECT 1 FROM finance_operator_grant_revocations r WHERE r.grant_id=g.id)
 )`, identity).Scan(&granted); err != nil {
		return "", err
	}
	if !granted {
		return "", ErrFinanceMFAInvalid
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO finance_operator_mfa_factors
 (identity_id,encrypted_secret,provisioned_by,approval_ref,provisioned_at)
 VALUES ($1::uuid,$2,$3,$4,$5)
 ON CONFLICT (identity_id) DO UPDATE SET encrypted_secret=EXCLUDED.encrypted_secret,
 last_accepted_step=-1,failure_count=0,locked_until=NULL,
 provisioned_by=EXCLUDED.provisioned_by,approval_ref=EXCLUDED.approval_ref,
 provisioned_at=EXCLUDED.provisioned_at,revoked_at=NULL
 WHERE finance_operator_mfa_factors.revoked_at IS NOT NULL`,
		identity, encrypted, approver, approval, now.UTC())
	if err != nil {
		return "", fmt.Errorf("persist finance MFA enrollment: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if affected != 1 {
		return "", ErrFinanceMFAExists
	}
	auditID, err := persistentid.New()
	if err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_records
 (id,actor_id,action,scope,resource_ref,new_state,reason,policy_version,correlation_id,occurred_at)
 VALUES ($1::uuid,$2,'finance.mfa.enroll','apgic/finance-operations',$3,'{"factor":"TOTP","state":"ENROLLED"}'::jsonb,$4,'finance-mfa-v1',$5,$6)`,
		auditID, approver, "identity/"+identity, "controlled enrollment: "+approval, "finance-mfa/"+auditID, now.UTC())
	if err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret), nil
}

// RevokeFinanceMFA disables a lost/stolen factor under named approval and
// commits its canonical audit fact atomically. No public revocation endpoint.
func (c *Checker) RevokeFinanceMFA(ctx context.Context, identity, approver, approval string, at time.Time) error {
	if c == nil || c.db == nil || !financeUUIDPattern.MatchString(identity) ||
		strings.TrimSpace(approver) == "" || strings.TrimSpace(approval) == "" {
		return ErrFinanceMFAInvalid
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE finance_operator_mfa_factors
 SET revoked_at=$2 WHERE identity_id=$1::uuid AND revoked_at IS NULL`, identity, at.UTC())
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrFinanceMFAInvalid
	}
	auditID, err := persistentid.New()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_records
 (id,actor_id,action,scope,resource_ref,new_state,reason,policy_version,correlation_id,occurred_at)
 VALUES ($1::uuid,$2,'finance.mfa.revoke','apgic/finance-operations',$3,'{"factor":"TOTP","state":"REVOKED"}'::jsonb,$4,'finance-mfa-v1',$5,$6)`,
		auditID, approver, "identity/"+identity, "approved revocation: "+approval, "finance-mfa/"+auditID, at.UTC())
	if err != nil {
		return err
	}
	return tx.Commit()
}

// VerifyFinanceMFA validates a real possession factor in one transaction.
// A failed response gives no hints about factor existence or exact state;
// repeated/cross-session codes cannot mint a second assertion.
func (c *Checker) VerifyFinanceMFA(ctx context.Context, identity, sessionRef, code string, key FinanceMFAKey, now time.Time) (bool, error) {
	if c == nil || c.db == nil || !financeUUIDPattern.MatchString(identity) || !strings.HasPrefix(sessionRef, "session:") || len(sessionRef) < 20 {
		return false, ErrFinanceMFAInvalid
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var granted bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(
  SELECT 1 FROM finance_operator_grants g WHERE g.identity_id=$1::uuid
  AND g.capability='FINANCE_DISCREPANCY_RESOLVE'
  AND NOT EXISTS(SELECT 1 FROM finance_operator_grant_revocations r WHERE r.grant_id=g.id)
 )`, identity).Scan(&granted); err != nil {
		return false, err
	}
	if !granted {
		return false, ErrFinanceMFAInvalid
	}
	var ciphertext []byte
	var lastStep int64
	var failed int
	var locked *time.Time
	err = tx.QueryRowContext(ctx, `SELECT encrypted_secret,last_accepted_step,failure_count,locked_until
 FROM finance_operator_mfa_factors WHERE identity_id=$1::uuid AND revoked_at IS NULL FOR UPDATE`, identity).Scan(&ciphertext, &lastStep, &failed, &locked)
	if err != nil {
		return false, ErrFinanceMFAInvalid
	}
	if locked != nil && locked.After(now.UTC()) {
		return false, ErrFinanceMFAInvalid
	}
	// After a completed cooldown the failure counter starts afresh.
	if locked != nil && !locked.After(now.UTC()) {
		failed = 0
	}
	secret, err := financeMFADecrypt(key, identity, ciphertext)
	if err != nil {
		return false, ErrFinanceMFAInvalid
	}
	step := financeMFAMatch(secret, code, now, lastStep)
	if step < 0 {
		failed++
		var blocked any
		if failed >= 5 {
			failed = 5
			blocked = now.Add(15 * time.Minute).UTC()
		}
		if _, err = tx.ExecContext(ctx, `UPDATE finance_operator_mfa_factors
   SET failure_count=$2,locked_until=$3 WHERE identity_id=$1::uuid`, identity, failed, blocked); err != nil {
			return false, err
		}
		if err = tx.Commit(); err != nil {
			return false, err
		}
		return false, ErrFinanceMFAInvalid
	}
	if _, err = tx.ExecContext(ctx, `UPDATE finance_operator_mfa_factors
 SET last_accepted_step=$2,failure_count=0,locked_until=NULL WHERE identity_id=$1::uuid`, identity, step); err != nil {
		return false, err
	}
	auditID, err := persistentid.New()
	if err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_records
 (id,actor_id,action,scope,resource_ref,new_state,reason,policy_version,correlation_id,occurred_at)
 VALUES($1::uuid,$2,'finance.mfa.verify','apgic/finance-operations',$3,'{"factor":"TOTP","verified":true}'::jsonb,$4,'finance-mfa-v1',$5,$6)`,
		auditID, identity, "identity/"+identity, "verified factor for "+sessionRef, "finance-mfa/"+auditID, now.UTC())
	if err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
