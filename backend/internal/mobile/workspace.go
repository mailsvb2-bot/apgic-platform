package mobile

import (
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/authz"
)

type WorkspaceKind string

const (
	WorkspaceClient       WorkspaceKind = "CLIENT"
	WorkspaceSpecialist   WorkspaceKind = "SPECIALIST"
	WorkspaceOrganization WorkspaceKind = "ORGANIZATION"
)

const (
	ReasonWorkspaceAllowed          = "WORKSPACE_ALLOWED"
	ReasonWorkspaceInvalid          = "WORKSPACE_INVALID"
	ReasonWorkspaceIdentityMismatch = "WORKSPACE_IDENTITY_MISMATCH"
	ReasonWorkspaceAuthorization    = "WORKSPACE_AUTHORIZATION_DENY"
)

type Workspace struct {
	ID         string
	IdentityID string
	TenantID   string
	Kind       WorkspaceKind
}

type WorkspaceResolution struct {
	Allowed    bool
	ReasonCode string
	Workspace  Workspace
}

func ResolveWorkspace(principal authz.Principal, workspace Workspace, now time.Time) WorkspaceResolution {
	deny := func(reason string) WorkspaceResolution {
		return WorkspaceResolution{Allowed: false, ReasonCode: reason}
	}

	if strings.TrimSpace(workspace.ID) == "" ||
		strings.TrimSpace(workspace.IdentityID) == "" ||
		strings.TrimSpace(workspace.TenantID) == "" ||
		now.IsZero() ||
		!validWorkspaceKind(workspace.Kind) {
		return deny(ReasonWorkspaceInvalid)
	}
	if workspace.IdentityID != principal.ID {
		return deny(ReasonWorkspaceIdentityMismatch)
	}

	result := authz.Authorize(authz.Input{
		Principal: principal,
		Resource: authz.ResourceRef{
			ID:       workspace.ID,
			TenantID: workspace.TenantID,
		},
		Action: "workspace.open",
		Risk:   authz.RiskNormal,
		Now:    now,
	})
	if result.Decision != authz.Allow {
		return deny(ReasonWorkspaceAuthorization + ":" + result.ReasonCode)
	}

	return WorkspaceResolution{
		Allowed:    true,
		ReasonCode: ReasonWorkspaceAllowed,
		Workspace:  workspace,
	}
}

func validWorkspaceKind(kind WorkspaceKind) bool {
	switch kind {
	case WorkspaceClient, WorkspaceSpecialist, WorkspaceOrganization:
		return true
	default:
		return false
	}
}
