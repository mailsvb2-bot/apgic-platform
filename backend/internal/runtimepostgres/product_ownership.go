package runtimepostgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/audit"
)

var ErrProductNotFound = errors.New("product not found")

func (c *Checker) IdentityOwnedProductCommercialOwner(productID, identityID string) (string, bool, error) {
	if c == nil || c.db == nil {
		return "", false, errors.New("postgres checker is not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var commercialOwnerRef string
	if err := c.db.QueryRowContext(ctx,
		`SELECT commercial_owner_ref
		   FROM products
		  WHERE id = $1::uuid
		    AND owner_type = 'IDENTITY'
		    AND owner_id = $2::uuid`,
		productID, identityID,
	).Scan(&commercialOwnerRef); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read identity-owned product: %w", err)
	}
	return commercialOwnerRef, true, nil
}

func (c *Checker) UpdateIdentityOwnedProductCommercialOwner(
	productID, identityID, newCommercialOwnerRef string,
	record audit.Record,
) (string, error) {
	if c == nil || c.db == nil {
		return "", errors.New("postgres checker is not initialized")
	}
	newCommercialOwnerRef = strings.TrimSpace(newCommercialOwnerRef)
	if newCommercialOwnerRef == "" {
		return "", errors.New("commercial owner ref is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin product ownership update: %w", err)
	}
	defer tx.Rollback()

	var oldCommercialOwnerRef string
	if err := tx.QueryRowContext(ctx,
		`SELECT commercial_owner_ref
		   FROM products
		  WHERE id = $1::uuid
		    AND owner_type = 'IDENTITY'
		    AND owner_id = $2::uuid
		  FOR UPDATE`,
		productID, identityID,
	).Scan(&oldCommercialOwnerRef); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrProductNotFound
		}
		return "", fmt.Errorf("lock identity-owned product: %w", err)
	}

	oldState, err := json.Marshal(map[string]string{"commercial_owner_ref": oldCommercialOwnerRef})
	if err != nil {
		return "", fmt.Errorf("encode old product ownership audit state: %w", err)
	}
	newState, err := json.Marshal(map[string]string{"commercial_owner_ref": newCommercialOwnerRef})
	if err != nil {
		return "", fmt.Errorf("encode new product ownership audit state: %w", err)
	}
	record.ResourceRef = productID
	record.OldState = oldState
	record.NewState = newState
	canonicalAudit, err := audit.New(record)
	if err != nil {
		return "", fmt.Errorf("validate product ownership audit record: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE products
		    SET commercial_owner_ref = $3
		  WHERE id = $1::uuid
		    AND owner_type = 'IDENTITY'
		    AND owner_id = $2::uuid`,
		productID, identityID, newCommercialOwnerRef,
	); err != nil {
		return "", fmt.Errorf("update product commercial owner: %w", err)
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO audit_records (
			id, actor_id, action, scope, resource_ref, old_state, new_state,
			reason, policy_version, correlation_id, occurred_at
		) VALUES (
			$1::uuid, $2, $3, $4, NULLIF($5, ''), $6::jsonb, $7::jsonb,
			$8, $9, NULLIF($10, ''), $11
		)`,
		canonicalAudit.ID,
		canonicalAudit.ActorID,
		canonicalAudit.Action,
		canonicalAudit.Scope,
		canonicalAudit.ResourceRef,
		nullableJSON(canonicalAudit.OldState),
		nullableJSON(canonicalAudit.NewState),
		canonicalAudit.Reason,
		canonicalAudit.PolicyVersion,
		canonicalAudit.CorrelationID,
		canonicalAudit.OccurredAt,
	); err != nil {
		return "", fmt.Errorf("append product ownership audit record: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit product ownership update: %w", err)
	}
	return oldCommercialOwnerRef, nil
}
