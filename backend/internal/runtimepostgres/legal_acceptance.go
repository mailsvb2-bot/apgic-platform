package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/legal"
)

func (c *Checker) RecordAcceptance(input legal.Acceptance) (legal.Acceptance, bool, error) {
	if c == nil || c.db == nil {
		return legal.Acceptance{}, false, errors.New("postgres checker is not initialized")
	}
	canonical, err := legal.NewAcceptance(
		input.ID,
		input.IdentityID,
		input.DocumentID,
		input.DocumentVersion,
		input.EvidenceHash,
		input.AcceptedAt,
	)
	if err != nil {
		return legal.Acceptance{}, false, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return legal.Acceptance{}, false, fmt.Errorf("begin legal acceptance: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO identities (id)
		 VALUES ($1::uuid)
		 ON CONFLICT (id) DO NOTHING`,
		canonical.IdentityID,
	); err != nil {
		return legal.Acceptance{}, false, fmt.Errorf("ensure acceptance identity: %w", err)
	}

	var insertedID string
	err = tx.QueryRowContext(ctx,
		`INSERT INTO legal_acceptances (
			id, identity_id, document_id, document_version, evidence_hash, accepted_at
		) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6)
		ON CONFLICT (identity_id, document_id, document_version, evidence_hash)
		DO NOTHING
		RETURNING id::text`,
		canonical.ID,
		canonical.IdentityID,
		canonical.DocumentID,
		canonical.DocumentVersion,
		canonical.EvidenceHash,
		canonical.AcceptedAt,
	).Scan(&insertedID)
	if err == nil {
		canonical.ID = insertedID
		if err := tx.Commit(); err != nil {
			return legal.Acceptance{}, false, fmt.Errorf("commit legal acceptance: %w", err)
		}
		return canonical, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return legal.Acceptance{}, false, fmt.Errorf("persist legal acceptance: %w", err)
	}

	var existing legal.Acceptance
	if err := tx.QueryRowContext(ctx,
		`SELECT id::text, identity_id::text, document_id, document_version, evidence_hash, accepted_at
		   FROM legal_acceptances
		  WHERE identity_id = $1::uuid
		    AND document_id = $2
		    AND document_version = $3
		    AND evidence_hash = $4`,
		canonical.IdentityID,
		canonical.DocumentID,
		canonical.DocumentVersion,
		canonical.EvidenceHash,
	).Scan(
		&existing.ID,
		&existing.IdentityID,
		&existing.DocumentID,
		&existing.DocumentVersion,
		&existing.EvidenceHash,
		&existing.AcceptedAt,
	); err != nil {
		return legal.Acceptance{}, false, fmt.Errorf("read idempotent legal acceptance: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return legal.Acceptance{}, false, fmt.Errorf("commit idempotent legal acceptance: %w", err)
	}
	return existing, true, nil
}

func (c *Checker) HasAcceptance(identityID, documentID, documentVersion string) (bool, error) {
	if c == nil || c.db == nil {
		return false, errors.New("postgres checker is not initialized")
	}
	if strings.TrimSpace(identityID) == "" ||
		strings.TrimSpace(documentID) == "" ||
		strings.TrimSpace(documentVersion) == "" {
		return false, legal.ErrInvalidAcceptance
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var exists bool
	if err := c.db.QueryRowContext(ctx,
		`SELECT EXISTS (
			SELECT 1
			  FROM legal_acceptances
			 WHERE identity_id = $1::uuid
			   AND document_id = $2
			   AND document_version = $3
		)`,
		identityID,
		documentID,
		documentVersion,
	).Scan(&exists); err != nil {
		return false, fmt.Errorf("read legal acceptance: %w", err)
	}
	return exists, nil
}
