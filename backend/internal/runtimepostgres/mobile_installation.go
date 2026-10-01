package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/mobile"
)

func (c *Checker) RegisterInstallation(input mobile.ClientInstallation) (mobile.ClientInstallation, bool, error) {
	if c == nil || c.db == nil {
		return mobile.ClientInstallation{}, false, errors.New("postgres checker is not initialized")
	}
	canonical, err := mobile.NewInstallation(input.ID, input.IdentityID, input.Platform, input.PushEndpoint, input.UpdatedAt)
	if err != nil {
		return mobile.ClientInstallation{}, false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return mobile.ClientInstallation{}, false, fmt.Errorf("begin mobile installation registration: %w", err)
	}
	defer tx.Rollback()
	if err := lockMobileInstallationKeyTx(ctx, tx, "installation:"+canonical.ID); err != nil {
		return mobile.ClientInstallation{}, false, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO identities (id) VALUES ($1::uuid) ON CONFLICT (id) DO NOTHING", canonical.IdentityID); err != nil {
		return mobile.ClientInstallation{}, false, fmt.Errorf("ensure installation identity: %w", err)
	}
	existing, found, err := installationByIDTx(ctx, tx, canonical.ID)
	if err != nil {
		return mobile.ClientInstallation{}, false, err
	}
	if found {
		if existing.IdentityID != canonical.IdentityID || existing.Platform != canonical.Platform || existing.State != mobile.InstallationActive || existing.PushEndpoint != canonical.PushEndpoint {
			return mobile.ClientInstallation{}, false, mobile.ErrInvalidInstallation
		}
		if err := tx.Commit(); err != nil {
			return mobile.ClientInstallation{}, false, fmt.Errorf("commit idempotent installation registration: %w", err)
		}
		return existing, true, nil
	}
	if err := revokeSameIdentityEndpointTx(ctx, tx, canonical.IdentityID, canonical.ID, canonical.PushEndpoint, canonical.UpdatedAt); err != nil {
		return mobile.ClientInstallation{}, false, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO client_installations (id, identity_id, platform, push_endpoint, push_generation, state, updated_at) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7)", canonical.ID, canonical.IdentityID, canonical.Platform, canonical.PushEndpoint, canonical.PushGeneration, string(canonical.State), canonical.UpdatedAt); err != nil {
		return mobile.ClientInstallation{}, false, fmt.Errorf("persist client installation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return mobile.ClientInstallation{}, false, fmt.Errorf("commit client installation: %w", err)
	}
	return canonical, false, nil
}

func (c *Checker) RotateInstallationPushEndpoint(identityID, installationID, endpoint string, now time.Time) (mobile.ClientInstallation, bool, error) {
	if c == nil || c.db == nil {
		return mobile.ClientInstallation{}, false, errors.New("postgres checker is not initialized")
	}
	identityID = strings.TrimSpace(identityID)
	installationID = strings.TrimSpace(installationID)
	endpoint = strings.TrimSpace(endpoint)
	if identityID == "" || installationID == "" || endpoint == "" || now.IsZero() {
		return mobile.ClientInstallation{}, false, mobile.ErrInvalidInstallation
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return mobile.ClientInstallation{}, false, fmt.Errorf("begin push rotation: %w", err)
	}
	defer tx.Rollback()
	if err := lockMobileInstallationKeyTx(ctx, tx, "installation:"+installationID); err != nil {
		return mobile.ClientInstallation{}, false, err
	}
	current, found, err := installationByIDTx(ctx, tx, installationID)
	if err != nil {
		return mobile.ClientInstallation{}, false, err
	}
	if !found || current.IdentityID != identityID {
		return mobile.ClientInstallation{}, false, mobile.ErrInstallationNotFound
	}
	if current.State != mobile.InstallationActive {
		return mobile.ClientInstallation{}, false, mobile.ErrInvalidInstallation
	}
	if current.PushEndpoint == endpoint {
		if err := tx.Commit(); err != nil {
			return mobile.ClientInstallation{}, false, fmt.Errorf("commit idempotent push rotation: %w", err)
		}
		return current, true, nil
	}
	if err := revokeSameIdentityEndpointTx(ctx, tx, identityID, installationID, endpoint, now); err != nil {
		return mobile.ClientInstallation{}, false, err
	}
	if err := current.RotatePushEndpoint(endpoint, now); err != nil {
		return mobile.ClientInstallation{}, false, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE client_installations SET push_endpoint = $3, push_generation = $4, updated_at = $5 WHERE id = $1::uuid AND identity_id = $2::uuid AND state = 'ACTIVE'", current.ID, current.IdentityID, current.PushEndpoint, current.PushGeneration, current.UpdatedAt); err != nil {
		return mobile.ClientInstallation{}, false, fmt.Errorf("rotate installation push endpoint: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return mobile.ClientInstallation{}, false, fmt.Errorf("commit push rotation: %w", err)
	}
	return current, false, nil
}

func (c *Checker) RevokeInstallation(identityID, installationID string, now time.Time) (mobile.ClientInstallation, bool, error) {
	if c == nil || c.db == nil {
		return mobile.ClientInstallation{}, false, errors.New("postgres checker is not initialized")
	}
	identityID = strings.TrimSpace(identityID)
	installationID = strings.TrimSpace(installationID)
	if identityID == "" || installationID == "" || now.IsZero() {
		return mobile.ClientInstallation{}, false, mobile.ErrInvalidInstallation
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return mobile.ClientInstallation{}, false, fmt.Errorf("begin installation revoke: %w", err)
	}
	defer tx.Rollback()
	if err := lockMobileInstallationKeyTx(ctx, tx, "installation:"+installationID); err != nil {
		return mobile.ClientInstallation{}, false, err
	}
	current, found, err := installationByIDTx(ctx, tx, installationID)
	if err != nil {
		return mobile.ClientInstallation{}, false, err
	}
	if !found || current.IdentityID != identityID {
		return mobile.ClientInstallation{}, false, mobile.ErrInstallationNotFound
	}
	if current.State == mobile.InstallationRevoked {
		if err := tx.Commit(); err != nil {
			return mobile.ClientInstallation{}, false, fmt.Errorf("commit idempotent installation revoke: %w", err)
		}
		return current, true, nil
	}
	if err := current.Revoke(now); err != nil {
		return mobile.ClientInstallation{}, false, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE client_installations SET state = 'REVOKED', push_endpoint = NULL, updated_at = $3 WHERE id = $1::uuid AND identity_id = $2::uuid", current.ID, current.IdentityID, current.UpdatedAt); err != nil {
		return mobile.ClientInstallation{}, false, fmt.Errorf("revoke client installation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return mobile.ClientInstallation{}, false, fmt.Errorf("commit installation revoke: %w", err)
	}
	return current, false, nil
}

func (c *Checker) ListInstallations(identityID string) ([]mobile.ClientInstallation, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("postgres checker is not initialized")
	}
	identityID = strings.TrimSpace(identityID)
	if identityID == "" {
		return nil, mobile.ErrInvalidInstallation
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rows, err := c.db.QueryContext(ctx, "SELECT id::text, identity_id::text, platform, push_endpoint, push_generation, state, updated_at FROM client_installations WHERE identity_id = $1::uuid ORDER BY updated_at, id", identityID)
	if err != nil {
		return nil, fmt.Errorf("list client installations: %w", err)
	}
	defer rows.Close()
	values := make([]mobile.ClientInstallation, 0)
	for rows.Next() {
		value, err := scanInstallation(rows)
		if err != nil {
			return nil, fmt.Errorf("scan client installation: %w", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list client installations: %w", err)
	}
	return values, nil
}

type installationScanner interface {
	Scan(dest ...any) error
}

func scanInstallation(scanner installationScanner) (mobile.ClientInstallation, error) {
	var value mobile.ClientInstallation
	var endpoint sql.NullString
	var state string
	if err := scanner.Scan(&value.ID, &value.IdentityID, &value.Platform, &endpoint, &value.PushGeneration, &state, &value.UpdatedAt); err != nil {
		return mobile.ClientInstallation{}, err
	}
	if endpoint.Valid {
		value.PushEndpoint = endpoint.String
	}
	value.State = mobile.InstallationState(state)
	return value, nil
}

func installationByIDTx(ctx context.Context, tx *sql.Tx, installationID string) (mobile.ClientInstallation, bool, error) {
	value, err := scanInstallation(tx.QueryRowContext(ctx, "SELECT id::text, identity_id::text, platform, push_endpoint, push_generation, state, updated_at FROM client_installations WHERE id = $1::uuid FOR UPDATE", installationID))
	if errors.Is(err, sql.ErrNoRows) {
		return mobile.ClientInstallation{}, false, nil
	}
	if err != nil {
		return mobile.ClientInstallation{}, false, fmt.Errorf("read client installation: %w", err)
	}
	return value, true, nil
}

func revokeSameIdentityEndpointTx(ctx context.Context, tx *sql.Tx, identityID, installationID, endpoint string, now time.Time) error {
	if err := lockMobileInstallationKeyTx(ctx, tx, "push-endpoint:"+endpoint); err != nil {
		return err
	}
	var ownerInstallationID, ownerIdentityID string
	err := tx.QueryRowContext(ctx, "SELECT id::text, identity_id::text FROM client_installations WHERE push_endpoint = $1 AND state = 'ACTIVE' AND id <> $2::uuid FOR UPDATE", endpoint, installationID).Scan(&ownerInstallationID, &ownerIdentityID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read active push endpoint owner: %w", err)
	}
	if ownerIdentityID != identityID {
		return mobile.ErrPushEndpointAlreadyInUse
	}
	if _, err := tx.ExecContext(ctx, "UPDATE client_installations SET state = 'REVOKED', push_endpoint = NULL, updated_at = $2 WHERE id = $1::uuid AND state = 'ACTIVE'", ownerInstallationID, now); err != nil {
		return fmt.Errorf("revoke superseded client installation: %w", err)
	}
	return nil
}

func lockMobileInstallationKeyTx(ctx context.Context, tx *sql.Tx, key string) error {
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext($1)::bigint)", key); err != nil {
		return fmt.Errorf("lock mobile installation key: %w", err)
	}
	return nil
}
