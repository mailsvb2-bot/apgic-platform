package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
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

func (c *Checker) UpdateIdentityOwnedProductCommercialOwner(productID, identityID, newCommercialOwnerRef string) (string, error) {
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
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit product ownership update: %w", err)
	}
	return oldCommercialOwnerRef, nil
}
