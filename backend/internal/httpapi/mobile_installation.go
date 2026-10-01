package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/mobile"
)

type registerMobileInstallationRequest struct {
	ID           string `json:"id"`
	Platform     string `json:"platform"`
	PushEndpoint string `json:"push_endpoint"`
}

type rotateMobilePushEndpointRequest struct {
	PushEndpoint string `json:"push_endpoint"`
}

type mobileInstallationListResponse struct {
	Installations []mobile.ClientInstallation `json:"installations"`
}

func registerMobileInstallations(
	mux *http.ServeMux,
	store mobile.InstallationStore,
	sessions *clientSessionManager,
	sessionConfigErr error,
	now func() time.Time,
) {
	mux.HandleFunc("POST /v1/mobile/installations", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "MOBILE_INSTALLATION_UNAVAILABLE", "Хранилище установки устройства недоступно.", false, nil)
			return
		}
		identityID, ok := requiredClientSessionIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		var body registerMobileInstallationRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			writeDemandError(w, r, http.StatusBadRequest, "MOBILE_INSTALLATION_INVALID", "Данные установки устройства некорректны.", false, nil)
			return
		}
		installation, err := mobile.NewInstallation(body.ID, identityID, body.Platform, body.PushEndpoint, now().UTC())
		if err != nil {
			writeDemandError(w, r, http.StatusBadRequest, "MOBILE_INSTALLATION_INVALID", "Данные установки устройства некорректны.", false, nil)
			return
		}
		persisted, idempotent, err := store.RegisterInstallation(installation)
		if err != nil {
			writeMobileInstallationError(w, r, err)
			return
		}
		status := http.StatusCreated
		if idempotent {
			status = http.StatusOK
		}
		writeJSON(w, status, persisted)
	})

	mux.HandleFunc("GET /v1/mobile/installations", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "MOBILE_INSTALLATION_UNAVAILABLE", "Хранилище установки устройства недоступно.", false, nil)
			return
		}
		identityID, ok := requiredClientSessionIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		installations, err := store.ListInstallations(identityID)
		if err != nil {
			writeMobileInstallationError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, mobileInstallationListResponse{Installations: installations})
	})

	mux.HandleFunc("PATCH /v1/mobile/installations/{installationID}/push-endpoint", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "MOBILE_INSTALLATION_UNAVAILABLE", "Хранилище установки устройства недоступно.", false, nil)
			return
		}
		identityID, ok := requiredClientSessionIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		installationID := strings.TrimSpace(r.PathValue("installationID"))
		var body rotateMobilePushEndpointRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil || installationID == "" {
			writeDemandError(w, r, http.StatusBadRequest, "MOBILE_INSTALLATION_INVALID", "Данные обновления push endpoint некорректны.", false, nil)
			return
		}
		persisted, _, err := store.RotateInstallationPushEndpoint(identityID, installationID, body.PushEndpoint, now().UTC())
		if err != nil {
			writeMobileInstallationError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, persisted)
	})

	mux.HandleFunc("POST /v1/mobile/installations/{installationID}/revoke", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "MOBILE_INSTALLATION_UNAVAILABLE", "Хранилище установки устройства недоступно.", false, nil)
			return
		}
		identityID, ok := requiredClientSessionIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		installationID := strings.TrimSpace(r.PathValue("installationID"))
		if installationID == "" {
			writeDemandError(w, r, http.StatusBadRequest, "MOBILE_INSTALLATION_INVALID", "Идентификатор установки устройства обязателен.", false, nil)
			return
		}
		persisted, _, err := store.RevokeInstallation(identityID, installationID, now().UTC())
		if err != nil {
			writeMobileInstallationError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, persisted)
	})
}

func writeMobileInstallationError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, mobile.ErrInvalidInstallation):
		writeDemandError(w, r, http.StatusBadRequest, "MOBILE_INSTALLATION_INVALID", "Операция с установкой устройства некорректна.", false, nil)
	case errors.Is(err, mobile.ErrInstallationNotFound):
		writeDemandError(w, r, http.StatusNotFound, "MOBILE_INSTALLATION_NOT_FOUND", "Установка устройства не найдена.", false, nil)
	case errors.Is(err, mobile.ErrPushEndpointAlreadyInUse):
		writeDemandError(w, r, http.StatusConflict, "MOBILE_PUSH_ENDPOINT_CONFLICT", "Push endpoint уже связан с другой Identity.", false, nil)
	default:
		writeDemandError(w, r, http.StatusServiceUnavailable, "MOBILE_INSTALLATION_STORAGE_FAILED", "Операция с установкой устройства временно недоступна.", true, nil)
	}
}
