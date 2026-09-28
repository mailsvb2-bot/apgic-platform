package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/identity"
)

var ErrIdentityNotFound = errors.New("identity not found")

func (c *Checker) GrantIdentityRole(identityID string, role identity.Role) (created bool, version uint64, err error) {
	if c == nil || c.db == nil {
		return false, 0, errors.New("postgres checker is not initialized")
	}
	if _, err := identity.New(identityID, role); err != nil {
		return false, 0, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return false, 0, fmt.Errorf("begin identity role grant: %w", err)
	}
	defer tx.Rollback()

	var currentVersion int64
	if err := tx.QueryRowContext(ctx,
		`SELECT version
		   FROM identities
		  WHERE id = $1::uuid
		  FOR UPDATE`,
		identityID,
	).Scan(&currentVersion); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, 0, ErrIdentityNotFound
		}
		return false, 0, fmt.Errorf("lock identity for role grant: %w", err)
	}
	if currentVersion <= 0 {
		return false, 0, errors.New("identity version must be positive")
	}

	result, err := tx.ExecContext(ctx,
		`INSERT INTO identity_roles (identity_id, role_code)
		 VALUES ($1::uuid, $2)
		 ON CONFLICT (identity_id, role_code) DO NOTHING`,
		identityID,
		string(role),
	)
	if err != nil {
		return false, 0, fmt.Errorf("grant identity role: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, 0, fmt.Errorf("read identity role grant rows: %w", err)
	}
	if affected == 0 {
		if err := tx.Commit(); err != nil {
			return false, 0, fmt.Errorf("commit idempotent identity role grant: %w", err)
		}
		return false, uint64(currentVersion), nil
	}
	if affected != 1 {
		return false, 0, fmt.Errorf("identity role grant affected %d rows", affected)
	}

	currentVersion++
	if _, err := tx.ExecContext(ctx,
		`UPDATE identities
		    SET version = $2
		  WHERE id = $1::uuid`,
		identityID,
		currentVersion,
	); err != nil {
		return false, 0, fmt.Errorf("advance identity version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, 0, fmt.Errorf("commit identity role grant: %w", err)
	}
	return true, uint64(currentVersion), nil
}

func (c *Checker) IdentityRoles(identityID string) ([]identity.Role, uint64, error) {
	if c == nil || c.db == nil {
		return nil, 0, errors.New("postgres checker is not initialized")
	}
	if _, err := identity.New(identityID); err != nil {
		return nil, 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  true,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("begin identity role snapshot: %w", err)
	}
	defer tx.Rollback()

	var version int64
	if err := tx.QueryRowContext(ctx,
		`SELECT version FROM identities WHERE id = $1::uuid`,
		identityID,
	).Scan(&version); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, 0, ErrIdentityNotFound
		}
		return nil, 0, fmt.Errorf("read identity version: %w", err)
	}
	rows, err := tx.QueryContext(ctx,
		`SELECT role_code
		   FROM identity_roles
		  WHERE identity_id = $1::uuid
		  ORDER BY role_code`,
		identityID,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("read identity roles: %w", err)
	}

	roles := make([]identity.Role, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			_ = rows.Close()
			return nil, 0, fmt.Errorf("scan identity role: %w", err)
		}
		role := identity.Role(raw)
		if _, err := identity.New(identityID, role); err != nil {
			_ = rows.Close()
			return nil, 0, fmt.Errorf("invalid durable identity role %q: %w", raw, err)
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, 0, fmt.Errorf("iterate identity roles: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, 0, fmt.Errorf("close identity roles snapshot: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, fmt.Errorf("commit identity role snapshot: %w", err)
	}
	return roles, uint64(version), nil
}
