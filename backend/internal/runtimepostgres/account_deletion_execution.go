package runtimepostgres

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/privacy"
)

var ErrDeletionManualReviewRequired = errors.New("deletion requires resolution of ongoing bookings or privileged roles")

// AdvanceApprovedDeletion applies local erasure and creates durable provider jobs
// in the SAME transaction as the canonical lifecycle transition. No caller can
// mark erasure complete by sending a client-supplied policy or evidence ref.
func (c *Checker) AdvanceApprovedDeletion(ctx context.Context, signedPolicy []byte, policyKey ed25519.PublicKey, expectedVersion string) (bool, error) {
	if c == nil || c.db == nil {
		return false, errors.New("postgres unavailable")
	}
	policy, err := privacy.VerifyDeletionPolicy(signedPolicy, policyKey, expectedVersion, time.Now().UTC())
	if err != nil {
		return false, err
	}
	hash := sha256.Sum256(signedPolicy)
	approvalHash := fmt.Sprintf("%x", hash)
	snapshot, err := json.Marshal(policy)
	if err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var requestID, owner string
	err = tx.QueryRowContext(ctx, `SELECT id::text, identity_id::text
		FROM delete_account_requests
		WHERE state='IDENTITY_RECONFIRMED'
		  AND NOT EXISTS(SELECT 1 FROM deletion_processing_holds h
		      WHERE h.request_id=delete_account_requests.id AND h.retry_after>now())
		ORDER BY created_at, id LIMIT 1 FOR UPDATE SKIP LOCKED`).
		Scan(&requestID, &owner)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// Other canonical owners may require handling legal duties before removing
	// access. Do not orphan paid appointments or active specialist identities.
	var blocked bool
	err = tx.QueryRowContext(ctx, `SELECT
		EXISTS(SELECT 1 FROM specialist_profiles WHERE identity_id=$1::uuid)
		OR EXISTS(SELECT 1 FROM organization_memberships WHERE identity_id=$1::uuid)
		OR EXISTS(SELECT 1 FROM bookings
		          WHERE client_identity_id=$1::uuid
		            AND state IN ('HELD','PENDING_PAYMENT','CONFIRMED'))`, owner).Scan(&blocked)
	if err != nil {
		return false, err
	}
	if blocked {
		_, err = tx.ExecContext(ctx, `INSERT INTO deletion_processing_holds
			(request_id,reason_code,retry_after)
			VALUES($1::uuid,'ACTIVE_OBLIGATION_REVIEW',now()+interval '1 hour')
			ON CONFLICT(request_id) DO UPDATE SET
			    retry_after=EXCLUDED.retry_after,updated_at=now()`, requestID)
		if err != nil {
			return false, err
		}
		if err = tx.Commit(); err != nil {
			return false, err
		}
		return true, nil
	}
	_, err = tx.ExecContext(ctx, `UPDATE delete_account_requests
		SET retention_snapshot=$2::jsonb, retained_reason=$3,
		    state='RETENTION_CLASSIFIED', updated_at=now()
		WHERE id=$1::uuid AND state='IDENTITY_RECONFIRMED'`,
		requestID, string(snapshot), policy.RetentionReason)
	if err != nil {
		return false, fmt.Errorf("classify deletion: %w", err)
	}
	prefs, err := tx.ExecContext(ctx, `DELETE FROM client_personal_preferences
		WHERE identity_id=$1::uuid`, owner)
	if err != nil {
		return false, fmt.Errorf("erase owned preferences: %w", err)
	}
	// PostgreSQL owns the FK-aware erasure: physical removal when safe,
	// privacy-field scrub and unlink only when immutable bookings reference it.
	var nIntents int64
	err = tx.QueryRowContext(ctx, `SELECT apgic_erase_owned_help_intents($1::uuid)`, owner).Scan(&nIntents)
	if err != nil {
		return false, fmt.Errorf("erase owned help intents: %w", err)
	}
	creds, err := tx.ExecContext(ctx, `DELETE FROM finance_operator_mfa_factors
		WHERE identity_id=$1::uuid`, owner)
	if err != nil {
		return false, fmt.Errorf("erase owned authentication secret: %w", err)
	}
	deletionFactor, err := tx.ExecContext(ctx, `DELETE FROM client_deletion_mfa_factors
		WHERE identity_id=$1::uuid`, owner)
	if err != nil {
		return false, fmt.Errorf("erase owned deletion possession factor: %w", err)
	}
	// Push endpoints are direct device identifiers and delivery credentials.
	// Revoke them in the same transaction as profile erasure.
	installations, err := tx.ExecContext(ctx, `UPDATE client_installations
		SET state='REVOKED', push_endpoint=NULL,
		    push_generation=push_generation+1, updated_at=now()
		WHERE identity_id=$1::uuid AND (state<>'REVOKED' OR push_endpoint IS NOT NULL)`, owner)
	if err != nil {
		return false, fmt.Errorf("revoke client push installations: %w", err)
	}
	var residue bool
	err = tx.QueryRowContext(ctx, `SELECT
		EXISTS(SELECT 1 FROM client_personal_preferences WHERE identity_id=$1::uuid)
		OR EXISTS(SELECT 1 FROM finance_operator_mfa_factors WHERE identity_id=$1::uuid)
		OR EXISTS(SELECT 1 FROM client_deletion_mfa_factors WHERE identity_id=$1::uuid)
		OR EXISTS(SELECT 1 FROM client_installations WHERE identity_id=$1::uuid
			AND (state<>'REVOKED' OR push_endpoint IS NOT NULL))
		OR EXISTS(SELECT 1 FROM help_intents WHERE identity_id=$1::uuid)`, owner).Scan(&residue)
	if err != nil || residue {
		return false, fmt.Errorf("post-erasure verification failed: residual=%v err=%w", residue, err)
	}
	nPrefs, _ := prefs.RowsAffected()
	nCreds, _ := creds.RowsAffected()
	nDeletionFactor, _ := deletionFactor.RowsAffected()
	nCreds += nDeletionFactor
	nInstallations, _ := installations.RowsAffected()
	nCreds += nInstallations
	_, err = tx.ExecContext(ctx, `INSERT INTO deletion_local_erasure_receipts
		(request_id,policy_version,approval_sha256,preferences_erased,
		 help_intents_anonymized,credentials_erased,no_external_data)
		VALUES($1::uuid,$2,$3,$4,$5,$6,$7)`,
		requestID, policy.Version, approvalHash, nPrefs, nIntents, nCreds, policy.NoExternalData)
	if err != nil {
		return false, fmt.Errorf("persist local deletion proof: %w", err)
	}
	for _, ref := range policy.ProviderRefs {
		jobID, e := persistentid.New()
		if e != nil {
			return false, e
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO provider_erasure_jobs
			(id,delete_request_id,provider_ref,state,updated_at)
			VALUES($1::uuid,$2::uuid,$3,'PENDING',now())`,
			jobID, requestID, ref)
		if e != nil {
			return false, fmt.Errorf("create erasure job: %w", e)
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO deletion_provider_dispatch
			(request_id,provider_ref) VALUES($1::uuid,$2)`,
			requestID, ref)
		if e != nil {
			return false, fmt.Errorf("create durable erasure delivery: %w", e)
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE delete_account_requests
		SET state='PROVIDER_ERASURE_PENDING',updated_at=now()
		WHERE id=$1::uuid AND state='RETENTION_CLASSIFIED'`, requestID)
	if err != nil {
		return false, fmt.Errorf("mark provider deletion pending: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
