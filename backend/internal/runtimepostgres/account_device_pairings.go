package runtimepostgres

import (
 "context"
 "database/sql"
 "errors"
 "time"
)

// CreateAccountDevicePair starts a five-minute browser-to-browser authorization.
// Only hashes are persisted. In particular the claim proof is never stored.
func (c *Checker) CreateAccountDevicePair(id, codeDigest, proofDigest string, expires time.Time) error {
 if c == nil || c.db == nil { return errors.New("storage unavailable") }
 ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second); defer cancel()
 _, err := c.db.ExecContext(ctx, `INSERT INTO account_device_pairings(id,code_digest,proof_digest,expires_at)
 VALUES($1::uuid,$2,$3,$4)`, id, codeDigest, proofDigest, expires)
 return err
}

// Approval only comes from an already trusted first-party identity session.
func (c *Checker) ApproveAccountDevicePair(codeDigest, identity string) (bool, error) {
 if c == nil || c.db == nil { return false, errors.New("storage unavailable") }
 ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second); defer cancel()
 result, err := c.db.ExecContext(ctx, `UPDATE account_device_pairings
 SET identity_id=$2::uuid, approved_at=now()
 WHERE code_digest=$1 AND identity_id IS NULL AND consumed_at IS NULL
 AND expires_at > now() AND EXISTS(SELECT 1 FROM identities WHERE id=$2::uuid)`, codeDigest, identity)
 if err != nil { return false, err }
 n, err := result.RowsAffected()
 return n == 1, err
}

// PostgreSQL's single UPDATE ... RETURNING is an atomic, one-time claim.
// Concurrency, process restarts and two API instances cannot double-claim.
func (c *Checker) ClaimAccountDevicePair(id, proofDigest string) (string, bool, error) {
 if c == nil || c.db == nil { return "", false, errors.New("storage unavailable") }
 ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second); defer cancel()
 var identity string
 err := c.db.QueryRowContext(ctx, `UPDATE account_device_pairings
 SET consumed_at=now()
 WHERE id=$1::uuid AND proof_digest=$2 AND identity_id IS NOT NULL
 AND approved_at IS NOT NULL AND consumed_at IS NULL AND expires_at > now()
 RETURNING identity_id::text`, id, proofDigest).Scan(&identity)
 if errors.Is(err, sql.ErrNoRows) { return "", false, nil }
 return identity, err == nil, err
}
