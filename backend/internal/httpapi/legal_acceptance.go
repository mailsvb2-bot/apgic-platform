package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/legal"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

type recordLegalAcceptanceRequest struct {
	DocumentID      string `json:"document_id"`
	DocumentVersion string `json:"document_version"`
	EvidenceHash    string `json:"evidence_hash"`
}

type legalAcceptanceStatusResponse struct {
	DocumentID      string `json:"document_id"`
	DocumentVersion string `json:"document_version"`
	Accepted        bool   `json:"accepted"`
}

func registerLegalAcceptance(
	mux *http.ServeMux,
	store legal.AcceptanceStore,
	sessions *clientSessionManager,
	sessionConfigErr error,
	now func() time.Time,
) {
	mux.HandleFunc("POST /v1/legal-acceptances", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "LEGAL_ACCEPTANCE_UNAVAILABLE", "Хранилище согласий недоступно.", false, nil)
			return
		}
		identityID, ok := requiredClientSessionIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		var body recordLegalAcceptanceRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			writeDemandError(w, r, http.StatusBadRequest, "LEGAL_ACCEPTANCE_INVALID", "Согласие не удалось прочитать.", false, nil)
			return
		}
		body.DocumentID = strings.TrimSpace(body.DocumentID)
		body.DocumentVersion = strings.TrimSpace(body.DocumentVersion)
		body.EvidenceHash = strings.TrimSpace(body.EvidenceHash)
		if body.DocumentID == "" || body.DocumentVersion == "" || body.EvidenceHash == "" {
			writeDemandError(w, r, http.StatusBadRequest, "LEGAL_ACCEPTANCE_INVALID", "Документ, версия и evidence hash обязательны.", false, nil)
			return
		}
		id, err := persistentid.New()
		if err != nil {
			writeDemandError(w, r, http.StatusInternalServerError, "LEGAL_ACCEPTANCE_ID_FAILED", "Не удалось создать запись согласия.", true, nil)
			return
		}
		acceptance, err := legal.NewAcceptance(
			id,
			identityID,
			body.DocumentID,
			body.DocumentVersion,
			body.EvidenceHash,
			now().UTC(),
		)
		if err != nil {
			writeDemandError(w, r, http.StatusBadRequest, "LEGAL_ACCEPTANCE_INVALID", "Согласие некорректно.", false, nil)
			return
		}
		persisted, idempotent, err := store.RecordAcceptance(acceptance)
		if err != nil {
			if errors.Is(err, legal.ErrInvalidAcceptance) {
				writeDemandError(w, r, http.StatusBadRequest, "LEGAL_ACCEPTANCE_INVALID", "Согласие некорректно.", false, nil)
				return
			}
			writeDemandError(w, r, http.StatusServiceUnavailable, "LEGAL_ACCEPTANCE_STORAGE_FAILED", "Не удалось сохранить согласие.", true, nil)
			return
		}
		status := http.StatusCreated
		if idempotent {
			status = http.StatusOK
		}
		writeJSON(w, status, persisted)
	})

	mux.HandleFunc("GET /v1/legal-acceptances/{documentID}/{documentVersion}", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "LEGAL_ACCEPTANCE_UNAVAILABLE", "Хранилище согласий недоступно.", false, nil)
			return
		}
		identityID, ok := requiredClientSessionIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		documentID := strings.TrimSpace(r.PathValue("documentID"))
		documentVersion := strings.TrimSpace(r.PathValue("documentVersion"))
		if documentID == "" || documentVersion == "" {
			writeDemandError(w, r, http.StatusBadRequest, "LEGAL_ACCEPTANCE_INVALID", "Документ и версия обязательны.", false, nil)
			return
		}
		accepted, err := store.HasAcceptance(identityID, documentID, documentVersion)
		if err != nil {
			if errors.Is(err, legal.ErrInvalidAcceptance) {
				writeDemandError(w, r, http.StatusBadRequest, "LEGAL_ACCEPTANCE_INVALID", "Параметры согласия некорректны.", false, nil)
				return
			}
			writeDemandError(w, r, http.StatusServiceUnavailable, "LEGAL_ACCEPTANCE_STORAGE_FAILED", "Не удалось проверить согласие.", true, nil)
			return
		}
		writeJSON(w, http.StatusOK, legalAcceptanceStatusResponse{
			DocumentID:      documentID,
			DocumentVersion: documentVersion,
			Accepted:        accepted,
		})
	})
}

func requiredClientSessionIdentity(
	w http.ResponseWriter,
	r *http.Request,
	sessions *clientSessionManager,
	sessionConfigErr error,
) (string, bool) {
	if sessionConfigErr != nil {
		writeDemandError(w, r, http.StatusServiceUnavailable, "CLIENT_SESSION_UNAVAILABLE", "Сессия клиента недоступна.", false, nil)
		return "", false
	}
	if sessions == nil {
		writeDemandError(w, r, http.StatusUnauthorized, "CLIENT_SESSION_REQUIRED", "Требуется сессия клиента.", false, nil)
		return "", false
	}
	identityID, err := sessions.identityFromRequest(r)
	if errors.Is(err, ErrClientSessionMissing) {
		writeDemandError(w, r, http.StatusUnauthorized, "CLIENT_SESSION_REQUIRED", "Требуется сессия клиента.", false, nil)
		return "", false
	}
	if err != nil {
		writeDemandError(w, r, http.StatusUnauthorized, "CLIENT_SESSION_INVALID", "Сессия клиента недействительна.", false, nil)
		return "", false
	}
	return identityID, true
}
