package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/organization"
)

type organizationRuntimeStore interface {
	CreateOrganization(identityID, name string) (organization.Snapshot, error)
	Organization(identityID, organizationID string) (organization.Snapshot, error)
	CreateOrganizationDirection(identityID, organizationID, name, directionType string) (organization.Snapshot, error)
	ArchiveOrganizationDirection(identityID, organizationID, directionID string) (organization.Snapshot, error)
}

type createOrganizationRequest struct {
	Name string `json:"name"`
}

type createOrganizationDirectionRequest struct {
	Name          string `json:"name"`
	DirectionType string `json:"direction_type"`
}

func registerOrganizationRuntime(
	mux *http.ServeMux,
	store organizationRuntimeStore,
	sessions *clientSessionManager,
	sessionConfigErr error,
) {
	mux.HandleFunc("POST /v1/organizations", func(w http.ResponseWriter, r *http.Request) {
		if store == nil || sessions == nil || sessionConfigErr != nil {
			writeOrganizationRuntimeError(w, r, http.StatusServiceUnavailable, "ORGANIZATION_STORE_UNAVAILABLE", "Организационный контур временно недоступен.")
			return
		}
		var body createOrganizationRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeOrganizationRuntimeError(w, r, http.StatusBadRequest, "ORGANIZATION_INVALID", "Организацию не удалось прочитать.")
			return
		}
		identityID, cookie, err := sessions.identityForCreate(r)
		if err != nil {
			writeOrganizationRuntimeError(w, r, http.StatusUnauthorized, "CLIENT_SESSION_INVALID", "Сессия пользователя недействительна.")
			return
		}
		snapshot, err := store.CreateOrganization(identityID, body.Name)
		if err != nil {
			writeOrganizationRuntimeFailure(w, r, err)
			return
		}
		if cookie != nil {
			http.SetCookie(w, cookie)
		}
		writeJSON(w, http.StatusCreated, snapshot)
	})

	mux.HandleFunc("GET /v1/organizations/{organizationID}", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeOrganizationRuntimeError(w, r, http.StatusServiceUnavailable, "ORGANIZATION_STORE_UNAVAILABLE", "Организационный контур временно недоступен.")
			return
		}
		identityID, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		snapshot, err := store.Organization(identityID, strings.TrimSpace(r.PathValue("organizationID")))
		if err != nil {
			writeOrganizationRuntimeFailure(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, snapshot)
	})

	mux.HandleFunc("POST /v1/organizations/{organizationID}/directions", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeOrganizationRuntimeError(w, r, http.StatusServiceUnavailable, "ORGANIZATION_STORE_UNAVAILABLE", "Организационный контур временно недоступен.")
			return
		}
		var body createOrganizationDirectionRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeOrganizationRuntimeError(w, r, http.StatusBadRequest, "ORGANIZATION_DIRECTION_INVALID", "Направление не удалось прочитать.")
			return
		}
		identityID, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		snapshot, err := store.CreateOrganizationDirection(
			identityID,
			strings.TrimSpace(r.PathValue("organizationID")),
			body.Name,
			body.DirectionType,
		)
		if err != nil {
			writeOrganizationRuntimeFailure(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, snapshot)
	})

	mux.HandleFunc("POST /v1/organizations/{organizationID}/directions/{directionID}/archive", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeOrganizationRuntimeError(w, r, http.StatusServiceUnavailable, "ORGANIZATION_STORE_UNAVAILABLE", "Организационный контур временно недоступен.")
			return
		}
		identityID, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		snapshot, err := store.ArchiveOrganizationDirection(
			identityID,
			strings.TrimSpace(r.PathValue("organizationID")),
			strings.TrimSpace(r.PathValue("directionID")),
		)
		if err != nil {
			writeOrganizationRuntimeFailure(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, snapshot)
	})
}

func writeOrganizationRuntimeFailure(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, organization.ErrOrganizationNotFound):
		writeOrganizationRuntimeError(w, r, http.StatusNotFound, "ORGANIZATION_NOT_FOUND", "Организация не найдена.")
	case errors.Is(err, organization.ErrDirectionNotFound):
		writeOrganizationRuntimeError(w, r, http.StatusNotFound, "ORGANIZATION_DIRECTION_NOT_FOUND", "Направление не найдено.")
	case errors.Is(err, organization.ErrOwnerRequired):
		writeOrganizationRuntimeError(w, r, http.StatusForbidden, "ORGANIZATION_OWNER_REQUIRED", "Для этого действия нужен активный владелец организации.")
	case errors.Is(err, organization.ErrInvalidOrganization):
		writeOrganizationRuntimeError(w, r, http.StatusBadRequest, "ORGANIZATION_INVALID", "Укажите название организации.")
	case errors.Is(err, organization.ErrInvalidDirection):
		writeOrganizationRuntimeError(w, r, http.StatusBadRequest, "ORGANIZATION_DIRECTION_INVALID", "Укажите название и тип направления.")
	default:
		writeOrganizationRuntimeError(w, r, http.StatusInternalServerError, "ORGANIZATION_INTERNAL_ERROR", "Организационный контур не удалось обновить.")
	}
}

func writeOrganizationRuntimeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeDemandError(w, r, status, code, message, status >= http.StatusInternalServerError, nil)
}
