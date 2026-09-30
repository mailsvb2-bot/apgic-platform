package runtimepostgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/audit"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/commerce"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

func (c *Checker) CreateOrganizationProduct(
	identityID, organizationID string,
	input commerce.OrganizationProductDraft,
) (commerce.ProductSnapshot, error) {
	if c == nil || c.db == nil {
		return commerce.ProductSnapshot{}, errors.New("postgres checker is not initialized")
	}
	input, err := commerce.NormalizeOrganizationProductDraft(input)
	if err != nil {
		return commerce.ProductSnapshot{}, err
	}
	productID, err := persistentid.New()
	if err != nil {
		return commerce.ProductSnapshot{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), organizationWriteTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return commerce.ProductSnapshot{}, fmt.Errorf("begin organization product create: %w", err)
	}
	defer tx.Rollback()

	owner, err := activeOrganizationOwner(ctx, tx, identityID, organizationID)
	if err != nil {
		return commerce.ProductSnapshot{}, err
	}
	if !owner {
		return commerce.ProductSnapshot{}, commerce.ErrProductOwnerRequired
	}
	var directionStatus string
	if err := tx.QueryRowContext(ctx,
		`SELECT status
		   FROM organization_directions
		  WHERE id = $1::uuid
		    AND organization_id = $2::uuid`,
		input.DirectionID, organizationID,
	).Scan(&directionStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return commerce.ProductSnapshot{}, commerce.ErrProductDirectionInvalid
		}
		return commerce.ProductSnapshot{}, fmt.Errorf("read product direction: %w", err)
	}
	if directionStatus != "ACTIVE" {
		return commerce.ProductSnapshot{}, commerce.ErrProductDirectionInvalid
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO products (
			id, name, status, owner_type, owner_id, commercial_owner_ref,
			revenue_beneficiary_ref, author_refs, organization_direction_id
		 ) VALUES (
			$1::uuid, $2, 'DRAFT', 'ORGANIZATION', $3::uuid, $4, $5, $6, $7::uuid
		 )`,
		productID,
		input.Name,
		organizationID,
		input.CommercialOwnerRef,
		input.RevenueBeneficiaryRef,
		input.AuthorRefs,
		input.DirectionID,
	); err != nil {
		return commerce.ProductSnapshot{}, fmt.Errorf("create organization product: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return commerce.ProductSnapshot{}, fmt.Errorf("commit organization product create: %w", err)
	}
	return c.OrganizationProduct(identityID, organizationID, productID)
}

func (c *Checker) OrganizationProducts(identityID, organizationID string) ([]commerce.ProductSnapshot, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("postgres checker is not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), organizationWriteTimeout)
	defer cancel()

	rows, err := c.db.QueryContext(ctx,
		`SELECT p.id::text
		   FROM products p
		   JOIN organization_memberships m
		     ON m.organization_id = p.owner_id
		    AND m.identity_id = $1::uuid
		    AND m.status = 'ACTIVE'
		  WHERE p.owner_type = 'ORGANIZATION'
		    AND p.owner_id = $2::uuid
		  ORDER BY p.created_at, p.id`,
		identityID, organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("list organization products: %w", err)
	}
	defer rows.Close()

	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan organization product id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate organization products: %w", err)
	}

	products := make([]commerce.ProductSnapshot, 0, len(ids))
	for _, id := range ids {
		product, err := c.OrganizationProduct(identityID, organizationID, id)
		if err != nil {
			return nil, err
		}
		products = append(products, product)
	}
	return products, nil
}

func (c *Checker) OrganizationProduct(identityID, organizationID, productID string) (commerce.ProductSnapshot, error) {
	if c == nil || c.db == nil {
		return commerce.ProductSnapshot{}, errors.New("postgres checker is not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), organizationWriteTimeout)
	defer cancel()

	var product commerce.ProductSnapshot
	var status string
	var ownerType string
	var authorsJSON string
	var publishedAt sql.NullTime
	if err := c.db.QueryRowContext(ctx,
		`SELECT p.id::text, COALESCE(p.name, ''), p.status, p.owner_type, p.owner_id::text,
		        COALESCE(p.commercial_owner_ref, ''), COALESCE(to_json(p.author_refs)::text, '[]'),
		        COALESCE(p.revenue_beneficiary_ref, ''), COALESCE(p.organization_direction_id::text, ''), p.published_at
		   FROM products p
		   JOIN organization_memberships m
		     ON m.organization_id = p.owner_id
		    AND m.identity_id = $1::uuid
		    AND m.status = 'ACTIVE'
		  WHERE p.id = $2::uuid
		    AND p.owner_type = 'ORGANIZATION'
		    AND p.owner_id = $3::uuid`,
		identityID, productID, organizationID,
	).Scan(
		&product.ID,
		&product.Name,
		&status,
		&ownerType,
		&product.OwnerID,
		&product.CommercialOwnerRef,
		&authorsJSON,
		&product.RevenueBeneficiaryRef,
		&product.OrganizationDirectionID,
		&publishedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return commerce.ProductSnapshot{}, commerce.ErrProductNotFound
		}
		return commerce.ProductSnapshot{}, fmt.Errorf("read organization product: %w", err)
	}
	product.Status = commerce.ProductStatus(status)
	product.OwnerType = commerce.OwnerType(ownerType)
	if err := json.Unmarshal([]byte(authorsJSON), &product.AuthorRefs); err != nil {
		return commerce.ProductSnapshot{}, fmt.Errorf("decode organization product authors: %w", err)
	}
	if publishedAt.Valid {
		value := publishedAt.Time.UTC()
		product.PublishedAt = &value
	}
	return product, nil
}

func (c *Checker) PublishOrganizationProduct(
	identityID, organizationID, productID string,
	now time.Time,
) (commerce.ProductSnapshot, error) {
	if c == nil || c.db == nil {
		return commerce.ProductSnapshot{}, errors.New("postgres checker is not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), organizationWriteTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return commerce.ProductSnapshot{}, fmt.Errorf("begin organization product publish: %w", err)
	}
	defer tx.Rollback()

	owner, err := activeOrganizationOwner(ctx, tx, identityID, organizationID)
	if err != nil {
		return commerce.ProductSnapshot{}, err
	}
	if !owner {
		return commerce.ProductSnapshot{}, commerce.ErrProductOwnerRequired
	}

	var oldStatus, directionID string
	if err := tx.QueryRowContext(ctx,
		`SELECT status, COALESCE(organization_direction_id::text, '')
		   FROM products
		  WHERE id = $1::uuid
		    AND owner_type = 'ORGANIZATION'
		    AND owner_id = $2::uuid
		  FOR UPDATE`,
		productID, organizationID,
	).Scan(&oldStatus, &directionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return commerce.ProductSnapshot{}, commerce.ErrProductNotFound
		}
		return commerce.ProductSnapshot{}, fmt.Errorf("lock organization product: %w", err)
	}
	if oldStatus == string(commerce.ProductPublished) {
		return commerce.ProductSnapshot{}, commerce.ErrProductAlreadyPublished
	}
	if directionID == "" {
		return commerce.ProductSnapshot{}, commerce.ErrProductDirectionInvalid
	}
	var directionStatus string
	if err := tx.QueryRowContext(ctx,
		`SELECT status
		   FROM organization_directions
		  WHERE id = $1::uuid
		    AND organization_id = $2::uuid
		  FOR SHARE`,
		directionID, organizationID,
	).Scan(&directionStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return commerce.ProductSnapshot{}, commerce.ErrProductDirectionInvalid
		}
		return commerce.ProductSnapshot{}, fmt.Errorf("read product publish direction: %w", err)
	}
	if directionStatus != "ACTIVE" {
		return commerce.ProductSnapshot{}, commerce.ErrProductDirectionInvalid
	}

	publishedAt := now.UTC()
	if _, err := tx.ExecContext(ctx,
		`UPDATE products
		    SET status = 'PUBLISHED',
		        published_at = $3
		  WHERE id = $1::uuid
		    AND owner_id = $2::uuid`,
		productID, organizationID, publishedAt,
	); err != nil {
		return commerce.ProductSnapshot{}, fmt.Errorf("publish organization product: %w", err)
	}

	auditID, err := persistentid.New()
	if err != nil {
		return commerce.ProductSnapshot{}, err
	}
	oldState, _ := json.Marshal(map[string]string{"status": oldStatus})
	newState, _ := json.Marshal(map[string]string{"status": string(commerce.ProductPublished)})
	record, err := audit.New(audit.Record{
		ID:            auditID,
		ActorID:       identityID,
		Action:        "product.published",
		Scope:         organizationID,
		ResourceRef:   productID,
		OldState:      oldState,
		NewState:      newState,
		Reason:        "APGIC-PROD-001",
		PolicyVersion: "product-publication-v1",
		OccurredAt:    publishedAt,
	})
	if err != nil {
		return commerce.ProductSnapshot{}, fmt.Errorf("validate product publication audit: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO audit_records (
			id, actor_id, action, scope, resource_ref, old_state, new_state,
			reason, policy_version, occurred_at
		) VALUES ($1::uuid, $2, $3, $4, $5, $6::jsonb, $7::jsonb, $8, $9, $10)`,
		record.ID, record.ActorID, record.Action, record.Scope, record.ResourceRef,
		nullableJSON(record.OldState), nullableJSON(record.NewState), record.Reason,
		record.PolicyVersion, record.OccurredAt,
	); err != nil {
		return commerce.ProductSnapshot{}, fmt.Errorf("append product publication audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return commerce.ProductSnapshot{}, fmt.Errorf("commit organization product publish: %w", err)
	}
	return c.OrganizationProduct(identityID, organizationID, productID)
}
