package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/authz"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/mobile"
)

const maxWorkspaceIDLength = 256

type mobileWorkspaceStore interface {
	MobileWorkspaces(identityID string) ([]mobile.Workspace, error)
	MobileWorkspace(identityID, workspaceID string) (mobile.Workspace, bool, error)
}

type authorizedWorkspaceResponse struct {
	WorkspaceID           string               `json:"workspace_id"`
	IdentityID            string               `json:"identity_id"`
	TenantID              string               `json:"tenant_id"`
	Kind                  mobile.WorkspaceKind `json:"kind"`
	AuthorizationDecision string               `json:"authorization_decision"`
	ReasonCode            string               `json:"reason_code"`
}

type mobileWorkspaceListResponse struct {
	Workspaces []authorizedWorkspaceResponse `json:"workspaces"`
}

type mobileWorkspaceResolutionResponse struct {
	Allowed    bool                         `json:"allowed"`
	ReasonCode string                       `json:"reason_code"`
	Workspace  *authorizedWorkspaceResponse `json:"workspace,omitempty"`
}

func registerMobileWorkspaces(
	mux *http.ServeMux,
	store mobileWorkspaceStore,
	sessions *clientSessionManager,
	sessionConfigErr error,
	now func() time.Time,
) {
	mux.HandleFunc("GET /v1/mobile/workspaces", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "WORKSPACE_STORE_UNAVAILABLE", "Рабочие пространства временно недоступны.", true, nil)
			return
		}
		identityID, ok := requiredClientSessionIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		workspaces, err := store.MobileWorkspaces(identityID)
		if err != nil {
			writeMobileWorkspaceFailure(w, r, err)
			return
		}
		result := mobileWorkspaceListResponse{Workspaces: make([]authorizedWorkspaceResponse, 0, len(workspaces))}
		for _, workspace := range workspaces {
			resolution := resolveStoredWorkspace(identityID, workspace, now())
			if !resolution.Allowed {
				continue
			}
			result.Workspaces = append(result.Workspaces, authorizedWorkspaceView(resolution))
		}
		writeJSON(w, http.StatusOK, result)
	})

	mux.HandleFunc("GET /v1/mobile/workspaces/{workspaceID}", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "WORKSPACE_STORE_UNAVAILABLE", "Рабочие пространства временно недоступны.", true, nil)
			return
		}
		identityID, ok := requiredClientSessionIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		workspaceID := strings.TrimSpace(r.PathValue("workspaceID"))
		if workspaceID == "" || len(workspaceID) > maxWorkspaceIDLength {
			writeJSON(w, http.StatusOK, mobileWorkspaceResolutionResponse{
				Allowed:    false,
				ReasonCode: mobile.ReasonWorkspaceAuthorization,
			})
			return
		}
		workspace, found, err := store.MobileWorkspace(identityID, workspaceID)
		if err != nil {
			writeMobileWorkspaceFailure(w, r, err)
			return
		}
		if !found {
			writeJSON(w, http.StatusOK, mobileWorkspaceResolutionResponse{
				Allowed:    false,
				ReasonCode: mobile.ReasonWorkspaceAuthorization,
			})
			return
		}
		resolution := resolveStoredWorkspace(identityID, workspace, now())
		response := mobileWorkspaceResolutionResponse{
			Allowed:    resolution.Allowed,
			ReasonCode: resolution.ReasonCode,
		}
		if resolution.Allowed {
			view := authorizedWorkspaceView(resolution)
			response.Workspace = &view
		}
		writeJSON(w, http.StatusOK, response)
	})
}

func resolveStoredWorkspace(identityID string, workspace mobile.Workspace, now time.Time) mobile.WorkspaceResolution {
	return mobile.ResolveWorkspace(
		authz.Principal{
			ID:       identityID,
			TenantID: workspace.TenantID,
			Permissions: map[string]struct{}{
				"workspace.open": {},
			},
		},
		workspace,
		now,
	)
}

func authorizedWorkspaceView(resolution mobile.WorkspaceResolution) authorizedWorkspaceResponse {
	return authorizedWorkspaceResponse{
		WorkspaceID:           resolution.Workspace.ID,
		IdentityID:            resolution.Workspace.IdentityID,
		TenantID:              resolution.Workspace.TenantID,
		Kind:                  resolution.Workspace.Kind,
		AuthorizationDecision: "ALLOW",
		ReasonCode:            resolution.ReasonCode,
	}
}

func writeMobileWorkspaceFailure(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, mobile.ErrWorkspaceIdentityNotFound) {
		writeDemandError(w, r, http.StatusForbidden, mobile.ReasonWorkspaceAuthorization, "Рабочее пространство недоступно для этой Identity.", false, []string{mobile.ReasonWorkspaceAuthorization})
		return
	}
	writeDemandError(w, r, http.StatusServiceUnavailable, "WORKSPACE_STORE_UNAVAILABLE", "Рабочие пространства временно недоступны.", true, nil)
}
