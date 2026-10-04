package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/mobile"
)

const mobileWorkspaceReadTimeout = 3 * time.Second

func (c *Checker) MobileWorkspaces(identityID string) ([]mobile.Workspace, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("postgres checker is not initialized")
	}
	identityID = strings.TrimSpace(identityID)
	if identityID == "" {
		return nil, mobile.ErrWorkspaceIdentityNotFound
	}

	ctx, cancel := context.WithTimeout(context.Background(), mobileWorkspaceReadTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, fmt.Errorf("begin mobile workspace snapshot: %w", err)
	}
	defer tx.Rollback()

	var identityExists bool
	if err := tx.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM identities WHERE id = $1::uuid)",
		identityID,
	).Scan(&identityExists); err != nil {
		return nil, fmt.Errorf("read mobile workspace identity: %w", err)
	}
	if !identityExists {
		return nil, mobile.ErrWorkspaceIdentityNotFound
	}

	workspaces := []mobile.Workspace{{
		ID:         mobile.ClientWorkspaceID(identityID),
		IdentityID: identityID,
		TenantID:   identityID,
		Kind:       mobile.WorkspaceClient,
	}}

	var specialistID string
	err = tx.QueryRowContext(ctx,
		"SELECT id::text FROM specialist_profiles WHERE identity_id = $1::uuid",
		identityID,
	).Scan(&specialistID)
	switch {
	case err == nil:
		workspaces = append(workspaces, mobile.Workspace{
			ID:         mobile.SpecialistWorkspaceID(specialistID),
			IdentityID: identityID,
			TenantID:   identityID,
			Kind:       mobile.WorkspaceSpecialist,
		})
	case errors.Is(err, sql.ErrNoRows):
	default:
		return nil, fmt.Errorf("read specialist mobile workspace: %w", err)
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT o.id::text
		   FROM organizations o
		   JOIN organization_memberships m
		     ON m.organization_id = o.id
		    AND m.identity_id = $1::uuid
		    AND m.status = 'ACTIVE'
		  WHERE o.status = 'ACTIVE'
		  ORDER BY o.created_at, o.id`,
		identityID,
	)
	if err != nil {
		return nil, fmt.Errorf("read organization mobile workspaces: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var organizationID string
		if err := rows.Scan(&organizationID); err != nil {
			return nil, fmt.Errorf("scan organization mobile workspace: %w", err)
		}
		workspaces = append(workspaces, mobile.Workspace{
			ID:         mobile.OrganizationWorkspaceID(organizationID),
			IdentityID: identityID,
			TenantID:   organizationID,
			Kind:       mobile.WorkspaceOrganization,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate organization mobile workspaces: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit mobile workspace snapshot: %w", err)
	}
	sort.SliceStable(workspaces, func(i, j int) bool {
		if workspaces[i].Kind == workspaces[j].Kind {
			return workspaces[i].ID < workspaces[j].ID
		}
		return workspaceKindOrder(workspaces[i].Kind) < workspaceKindOrder(workspaces[j].Kind)
	})
	return workspaces, nil
}

func (c *Checker) MobileWorkspace(identityID, workspaceID string) (mobile.Workspace, bool, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return mobile.Workspace{}, false, nil
	}
	workspaces, err := c.MobileWorkspaces(identityID)
	if err != nil {
		return mobile.Workspace{}, false, err
	}
	for _, workspace := range workspaces {
		if workspace.ID == workspaceID {
			return workspace, true, nil
		}
	}
	return mobile.Workspace{}, false, nil
}

func workspaceKindOrder(kind mobile.WorkspaceKind) int {
	switch kind {
	case mobile.WorkspaceClient:
		return 0
	case mobile.WorkspaceSpecialist:
		return 1
	case mobile.WorkspaceOrganization:
		return 2
	default:
		return 3
	}
}
