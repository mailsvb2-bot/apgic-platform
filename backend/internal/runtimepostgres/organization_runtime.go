package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/organization"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

const organizationWriteTimeout = 3 * time.Second

type OrganizationRecord struct {
	ID         string                    `json:"id"`
	Name       string                    `json:"name"`
	Status     string                    `json:"status"`
	Directions []OrganizationDirection   `json:"directions"`
}

type OrganizationDirection struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name"`
	Type           string `json:"direction_type"`
	Status         string `json:"status"`
}

func (c *Checker) CreateOrganization(identityID, name string) (OrganizationRecord, error) {
	if c == nil || c.db == nil {
		return OrganizationRecord{}, errors.New("postgres checker is not initialized")
	}
	name, err := organization.NormalizeOrganizationName(name)
	if err != nil {
		return OrganizationRecord{}, err
	}
	orgID, err := persistentid.New()
	if err != nil {
		return OrganizationRecord{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), organizationWriteTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return OrganizationRecord{}, fmt.Errorf("begin organization create: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO identities (id) VALUES ($1::uuid) ON CONFLICT (id) DO NOTHING`,
		identityID,
	); err != nil {
		return OrganizationRecord{}, fmt.Errorf("ensure organization owner identity: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO organizations (id, name, status) VALUES ($1::uuid, $2, 'ACTIVE')`,
		orgID, name,
	); err != nil {
		return OrganizationRecord{}, fmt.Errorf("create organization: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO organization_memberships (organization_id, identity_id, status)
		 VALUES ($1::uuid, $2::uuid, 'ACTIVE')`,
		orgID, identityID,
	); err != nil {
		return OrganizationRecord{}, fmt.Errorf("create organization owner membership: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO organization_ownerships (organization_id, identity_id, status)
		 VALUES ($1::uuid, $2::uuid, 'ACTIVE')`,
		orgID, identityID,
	); err != nil {
		return OrganizationRecord{}, fmt.Errorf("create organization ownership: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return OrganizationRecord{}, fmt.Errorf("commit organization create: %w", err)
	}
	return c.Organization(identityID, orgID)
}

func (c *Checker) Organization(identityID, organizationID string) (OrganizationRecord, error) {
	if c == nil || c.db == nil {
		return OrganizationRecord{}, errors.New("postgres checker is not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), organizationWriteTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return OrganizationRecord{}, fmt.Errorf("begin organization snapshot: %w", err)
	}
	defer tx.Rollback()

	var record OrganizationRecord
	if err := tx.QueryRowContext(ctx,
		`SELECT o.id::text, o.name, o.status
		   FROM organizations o
		   JOIN organization_memberships m
		     ON m.organization_id = o.id
		    AND m.identity_id = $1::uuid
		    AND m.status = 'ACTIVE'
		  WHERE o.id = $2::uuid`,
		identityID, organizationID,
	).Scan(&record.ID, &record.Name, &record.Status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return OrganizationRecord{}, organization.ErrOrganizationNotFound
		}
		return OrganizationRecord{}, fmt.Errorf("read organization: %w", err)
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT id::text, organization_id::text, name, direction_type, status
		   FROM organization_directions
		  WHERE organization_id = $1::uuid
		  ORDER BY created_at, id`,
		organizationID,
	)
	if err != nil {
		return OrganizationRecord{}, fmt.Errorf("read organization directions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var direction OrganizationDirection
		if err := rows.Scan(
			&direction.ID,
			&direction.OrganizationID,
			&direction.Name,
			&direction.Type,
			&direction.Status,
		); err != nil {
			return OrganizationRecord{}, fmt.Errorf("scan organization direction: %w", err)
		}
		record.Directions = append(record.Directions, direction)
	}
	if err := rows.Err(); err != nil {
		return OrganizationRecord{}, fmt.Errorf("iterate organization directions: %w", err)
	}
	if record.Directions == nil {
		record.Directions = []OrganizationDirection{}
	}
	if err := tx.Commit(); err != nil {
		return OrganizationRecord{}, fmt.Errorf("commit organization snapshot: %w", err)
	}
	return record, nil
}

func (c *Checker) CreateOrganizationDirection(identityID, organizationID, name, directionType string) (OrganizationRecord, error) {
	if c == nil || c.db == nil {
		return OrganizationRecord{}, errors.New("postgres checker is not initialized")
	}
	name, directionType, err := organization.NormalizeDirection(name, directionType)
	if err != nil {
		return OrganizationRecord{}, err
	}
	directionID, err := persistentid.New()
	if err != nil {
		return OrganizationRecord{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), organizationWriteTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return OrganizationRecord{}, fmt.Errorf("begin organization direction create: %w", err)
	}
	defer tx.Rollback()

	owner, err := activeOrganizationOwner(ctx, tx, identityID, organizationID)
	if err != nil {
		return OrganizationRecord{}, err
	}
	if !owner {
		return OrganizationRecord{}, organization.ErrOwnerRequired
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO organization_directions (
			id, organization_id, name, status, direction_type
		 ) VALUES ($1::uuid, $2::uuid, $3, 'ACTIVE', $4)`,
		directionID, organizationID, name, directionType,
	); err != nil {
		return OrganizationRecord{}, fmt.Errorf("create organization direction: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return OrganizationRecord{}, fmt.Errorf("commit organization direction create: %w", err)
	}
	return c.Organization(identityID, organizationID)
}

func (c *Checker) ArchiveOrganizationDirection(identityID, organizationID, directionID string) (OrganizationRecord, error) {
	if c == nil || c.db == nil {
		return OrganizationRecord{}, errors.New("postgres checker is not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), organizationWriteTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return OrganizationRecord{}, fmt.Errorf("begin organization direction archive: %w", err)
	}
	defer tx.Rollback()

	owner, err := activeOrganizationOwner(ctx, tx, identityID, organizationID)
	if err != nil {
		return OrganizationRecord{}, err
	}
	if !owner {
		return OrganizationRecord{}, organization.ErrOwnerRequired
	}
	result, err := tx.ExecContext(ctx,
		`UPDATE organization_directions
		    SET status = 'ARCHIVED',
		        archived_at = COALESCE(archived_at, now())
		  WHERE id = $1::uuid
		    AND organization_id = $2::uuid`,
		directionID, organizationID,
	)
	if err != nil {
		return OrganizationRecord{}, fmt.Errorf("archive organization direction: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return OrganizationRecord{}, organization.ErrDirectionNotFound
	}
	if err := tx.Commit(); err != nil {
		return OrganizationRecord{}, fmt.Errorf("commit organization direction archive: %w", err)
	}
	return c.Organization(identityID, organizationID)
}

func activeOrganizationOwner(ctx context.Context, tx *sql.Tx, identityID, organizationID string) (bool, error) {
	var active bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (
			SELECT 1
			  FROM organization_ownerships
			 WHERE organization_id = $1::uuid
			   AND identity_id = $2::uuid
			   AND status = 'ACTIVE'
		)`,
		organizationID, identityID,
	).Scan(&active); err != nil {
		return false, fmt.Errorf("read organization ownership: %w", err)
	}
	return active, nil
}
