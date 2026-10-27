package runtimepostgres

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/privacy"
)

// ProviderErasureDelivery is an individually leased, retryable external effect.
type ProviderErasureDelivery struct {
	RequestID   string
	ProviderRef string
	IdentityID  string
	Attempt     int
}

func (c *Checker) ClaimProviderErasure(ctx context.Context) (*ProviderErasureDelivery, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("postgres unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var d ProviderErasureDelivery
	err = tx.QueryRowContext(ctx, `SELECT d.request_id::text,d.provider_ref,r.identity_id::text,d.attempts
		FROM deletion_provider_dispatch d
		JOIN provider_erasure_jobs p ON p.delete_request_id=d.request_id
			AND p.provider_ref=d.provider_ref
		JOIN delete_account_requests r ON r.id=d.request_id
		WHERE p.state='PENDING' AND r.state='PROVIDER_ERASURE_PENDING'
			AND d.next_attempt_at<=now()
			AND (d.lease_until IS NULL OR d.lease_until<now())
		ORDER BY d.next_attempt_at,d.request_id
		LIMIT 1 FOR UPDATE OF d SKIP LOCKED`).
		Scan(&d.RequestID, &d.ProviderRef, &d.IdentityID, &d.Attempt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d.Attempt++
	_, err = tx.ExecContext(ctx, `UPDATE deletion_provider_dispatch
		SET attempts=$3,lease_until=now()+interval '2 minutes',updated_at=now()
		WHERE request_id=$1::uuid AND provider_ref=$2`,
		d.RequestID, d.ProviderRef, d.Attempt)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &d, nil
}

// FailProviderErasure releases only the current attempt's lease; stale workers
// cannot undo another attempt or produce fake success.
func (c *Checker) FailProviderErasure(ctx context.Context, d ProviderErasureDelivery, reason string) error {
	if c == nil || c.db == nil {
		return errors.New("postgres unavailable")
	}
	if reason != "HTTP_UNAVAILABLE" && reason != "INVALID_PROVIDER_RECEIPT" && reason != "PROVIDER_NOT_CONFIGURED" {
		reason = "PROVIDER_ERASURE_FAILED"
	}
	attempt := d.Attempt
	if attempt < 1 {
		return errors.New("invalid dispatch attempt")
	}
	if attempt > 12 {
		attempt = 12
	}
	backoff := time.Duration(1<<uint(attempt)) * time.Second
	if backoff > time.Hour {
		backoff = time.Hour
	}
	_, err := c.db.ExecContext(ctx, `UPDATE deletion_provider_dispatch SET
		lease_until=NULL, next_attempt_at=$4, last_error_code=$5, updated_at=now()
		WHERE request_id=$1::uuid AND provider_ref=$2 AND attempts=$3
			AND lease_until IS NOT NULL`,
		d.RequestID, d.ProviderRef, d.Attempt, time.Now().UTC().Add(backoff), reason)
	return err
}

// SaveProviderErasure validates provider-controlled Ed25519 evidence before
// atomically closing its immutable erasure job and durable dispatch.
func (c *Checker) SaveProviderErasure(ctx context.Context, d ProviderErasureDelivery, signed []byte, publicKey ed25519.PublicKey) error {
	if c == nil || c.db == nil {
		return errors.New("postgres unavailable")
	}
	proof, digest, err := privacy.VerifyProviderErasure(signed, publicKey, d.RequestID, d.ProviderRef, d.IdentityID, time.Now().UTC())
	if err != nil {
		return err
	}
	var envelope privacy.SignedProviderErasureAttestation
	if err := json.Unmarshal(signed, &envelope); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var state string
	err = tx.QueryRowContext(ctx, `SELECT state FROM provider_erasure_jobs
		WHERE delete_request_id=$1::uuid AND provider_ref=$2 FOR UPDATE`,
		d.RequestID, d.ProviderRef).Scan(&state)
	if err != nil {
		return err
	}
	if state == "SUCCEEDED" {
		var existing string
		err = tx.QueryRowContext(ctx, `SELECT signed_payload_sha256
			FROM deletion_provider_verified_receipts
			WHERE request_id=$1::uuid AND provider_ref=$2`, d.RequestID, d.ProviderRef).Scan(&existing)
		if err != nil || existing != digest {
			return privacy.ErrProviderErasureUnverified
		}
		return tx.Commit()
	}
	if state != "PENDING" {
		return privacy.ErrProviderErasureUnverified
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO deletion_provider_verified_receipts
		(request_id,provider_ref,evidence_ref,signed_payload_sha256,signature_base64,verified_at)
		VALUES($1::uuid,$2,$3,$4,$5,now())`,
		d.RequestID, d.ProviderRef, proof.EvidenceRef, digest, envelope.Signature)
	if err != nil {
		return fmt.Errorf("persist independent erasure attestation: %w", err)
	}
	_, err = tx.ExecContext(ctx, `UPDATE provider_erasure_jobs
		SET state='SUCCEEDED',evidence_ref=$3,updated_at=now()
		WHERE delete_request_id=$1::uuid AND provider_ref=$2 AND state='PENDING'`,
		d.RequestID, d.ProviderRef, proof.EvidenceRef)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE deletion_provider_dispatch
		SET lease_until=NULL,next_attempt_at='infinity'::timestamptz,
		    last_error_code=NULL,updated_at=now()
		WHERE request_id=$1::uuid AND provider_ref=$2`, d.RequestID, d.ProviderRef)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// FinalizeVerifiedDeletion irrevocably revokes the owner in the same
// transaction as terminal status. PostgreSQL guards independently validate
// local receipts and signed external attestations.
func (c *Checker) FinalizeVerifiedDeletion(ctx context.Context) (bool, error) {
	if c == nil || c.db == nil {
		return false, errors.New("postgres unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var requestID, owner string
	err = tx.QueryRowContext(ctx, `SELECT r.id::text,r.identity_id::text
		FROM delete_account_requests r
		WHERE r.state='PROVIDER_ERASURE_PENDING'
		  AND EXISTS(SELECT 1 FROM deletion_local_erasure_receipts x WHERE x.request_id=r.id)
		  AND NOT EXISTS(SELECT 1 FROM provider_erasure_jobs p
			WHERE p.delete_request_id=r.id AND p.state<>'SUCCEEDED')
		  AND NOT EXISTS(SELECT 1 FROM provider_erasure_jobs p
			WHERE p.delete_request_id=r.id AND NOT EXISTS(
			  SELECT 1 FROM deletion_provider_verified_receipts v
			  WHERE v.request_id=p.delete_request_id AND v.provider_ref=p.provider_ref))
		ORDER BY r.created_at,r.id LIMIT 1 FOR UPDATE OF r SKIP LOCKED`).
		Scan(&requestID, &owner)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE delete_account_requests
		SET state='PARTIALLY_RETAINED_WITH_REASON',completed_at=now(),updated_at=now()
		WHERE id=$1::uuid AND state='PROVIDER_ERASURE_PENDING'
		  AND nullif(btrim(retained_reason),'') IS NOT NULL`, requestID)
	if err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO deletion_revoked_identities(identity_id,request_id)
		VALUES($1::uuid,$2::uuid) ON CONFLICT(identity_id) DO NOTHING`, owner, requestID)
	if err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (c *Checker) ClientIdentityDeleted(ctx context.Context, identity string) (bool, error) {
	if c == nil || c.db == nil {
		return false, errors.New("postgres unavailable")
	}
	var deleted bool
	err := c.db.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM deletion_revoked_identities WHERE identity_id=$1::uuid)`, identity).Scan(&deleted)
	return deleted, err
}
