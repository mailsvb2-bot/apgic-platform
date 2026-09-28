package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func (c *Checker) ActiveOrganizationMembership(identityID, organizationID string) (bool, error) {
	if c == nil || c.db == nil {
		return false, errors.New("postgres checker is not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var active bool
	if err := c.db.QueryRowContext(ctx,
		`SELECT EXISTS (
			SELECT 1
			  FROM organization_memberships
			 WHERE identity_id = $1::uuid
			   AND organization_id = $2::uuid
			   AND status = 'ACTIVE'
		)`,
		identityID, organizationID,
	).Scan(&active); err != nil {
		return false, fmt.Errorf("read organization membership: %w", err)
	}
	return active, nil
}

func (c *Checker) OrganizationPrivateName(organizationID string) (string, bool, error) {
	if c == nil || c.db == nil {
		return "", false, errors.New("postgres checker is not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var name string
	if err := c.db.QueryRowContext(ctx,
		`SELECT name
		   FROM organizations
		  WHERE id = $1::uuid
		    AND status = 'ACTIVE'`,
		organizationID,
	).Scan(&name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read private organization profile: %w", err)
	}
	return name, true, nil
}
