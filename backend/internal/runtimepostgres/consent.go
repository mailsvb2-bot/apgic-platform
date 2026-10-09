package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/privacy"
)

const consentSelect = "SELECT consent_id::text, subject_id::text, purpose, scope, policy_version, text_hash_or_version, granted_at, revoked_at, source, proof_metadata::text FROM consent_records"

func scanConsent(row *sql.Row) (privacy.ConsentRecord, error) {
	var record privacy.ConsentRecord
	var proof string
	var revoked sql.NullTime
	if err := row.Scan(&record.ID, &record.SubjectID, &record.Purpose, &record.Scope, &record.PolicyVersion, &record.TextHashOrVersion, &record.GrantedAt, &revoked, &record.Source, &proof); err != nil {
		return privacy.ConsentRecord{}, err
	}
	record.ProofMetadata = []byte(proof)
	if revoked.Valid {
		value := revoked.Time.UTC()
		record.RevokedAt = &value
	}
	return record, nil
}

func (c *Checker) RecordConsent(input privacy.ConsentRecord) (privacy.ConsentRecord, bool, error) {
	if c == nil || c.db == nil {
		return privacy.ConsentRecord{}, false, errors.New("postgres checker is not initialized")
	}
	canonical, err := privacy.NewConsentRecord(input)
	if err != nil {
		return privacy.ConsentRecord{}, false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return privacy.ConsentRecord{}, false, fmt.Errorf("begin consent grant: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "INSERT INTO identities (id) VALUES ($1::uuid) ON CONFLICT (id) DO NOTHING", canonical.SubjectID); err != nil {
		return privacy.ConsentRecord{}, false, fmt.Errorf("ensure consent subject: %w", err)
	}
	var existing privacy.ConsentRecord
	var proof string
	var revoked sql.NullTime
	err = tx.QueryRowContext(ctx, consentSelect+" WHERE subject_id=$1::uuid AND purpose=$2 AND scope=$3 AND revoked_at IS NULL", canonical.SubjectID, canonical.Purpose, canonical.Scope).
		Scan(&existing.ID, &existing.SubjectID, &existing.Purpose, &existing.Scope, &existing.PolicyVersion, &existing.TextHashOrVersion, &existing.GrantedAt, &revoked, &existing.Source, &proof)
	if err == nil {
		existing.ProofMetadata = []byte(proof)
		if err := tx.Commit(); err != nil {
			return privacy.ConsentRecord{}, false, err
		}
		return existing, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return privacy.ConsentRecord{}, false, fmt.Errorf("read active consent: %w", err)
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO consent_records (consent_id,subject_id,purpose,scope,policy_version,text_hash_or_version,granted_at,source,proof_metadata) VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9::jsonb)", canonical.ID, canonical.SubjectID, canonical.Purpose, canonical.Scope, canonical.PolicyVersion, canonical.TextHashOrVersion, canonical.GrantedAt, canonical.Source, string(canonical.ProofMetadata))
	if err != nil {
		return privacy.ConsentRecord{}, false, fmt.Errorf("persist consent: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return privacy.ConsentRecord{}, false, err
	}
	return canonical, false, nil
}

func (c *Checker) ActiveConsent(subjectID, purpose, scope string, at time.Time) (privacy.ConsentRecord, bool, error) {
	if c == nil || c.db == nil {
		return privacy.ConsentRecord{}, false, errors.New("postgres checker is not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	record, err := scanConsent(c.db.QueryRowContext(ctx, consentSelect+" WHERE subject_id=$1::uuid AND purpose=$2 AND scope=$3 AND granted_at <= $4 AND revoked_at IS NULL ORDER BY granted_at DESC LIMIT 1", subjectID, purpose, scope, at.UTC()))
	if errors.Is(err, sql.ErrNoRows) {
		return privacy.ConsentRecord{}, false, nil
	}
	if err != nil {
		return privacy.ConsentRecord{}, false, fmt.Errorf("read active consent: %w", err)
	}
	return record, true, nil
}

func (c *Checker) RevokeConsent(consentID, subjectID string, revokedAt time.Time) (privacy.ConsentRecord, bool, error) {
	if c == nil || c.db == nil {
		return privacy.ConsentRecord{}, false, errors.New("postgres checker is not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := c.db.ExecContext(ctx, "UPDATE consent_records SET revoked_at=$3 WHERE consent_id=$1::uuid AND subject_id=$2::uuid AND revoked_at IS NULL", consentID, subjectID, revokedAt.UTC())
	if err != nil {
		return privacy.ConsentRecord{}, false, fmt.Errorf("revoke consent: %w", err)
	}
	affected, _ := result.RowsAffected()
	record, err := scanConsent(c.db.QueryRowContext(ctx, consentSelect+" WHERE consent_id=$1::uuid AND subject_id=$2::uuid", consentID, subjectID))
	if errors.Is(err, sql.ErrNoRows) {
		return privacy.ConsentRecord{}, false, privacy.ErrConsentNotFound
	}
	if err != nil {
		return privacy.ConsentRecord{}, false, err
	}
	return record, affected == 0, nil
}
