package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/privacy"
)

type recordConsentRequest struct {
	Purpose           string          `json:"purpose"`
	Scope             string          `json:"scope"`
	PolicyVersion     string          `json:"policy_version"`
	TextHashOrVersion string          `json:"text_hash_or_version"`
	Source            string          `json:"source"`
	ProofMetadata     json.RawMessage `json:"proof_metadata"`
}

func registerConsents(
	mux *http.ServeMux,
	store privacy.ConsentStore,
	sessions *clientSessionManager,
	sessionConfigErr error,
	now func() time.Time,
) {
	mux.HandleFunc("POST /v1/consents", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "CONSENT_STORE_UNAVAILABLE", "Хранилище согласий недоступно.", true, nil)
			return
		}
		identityID, ok := requiredClientSessionIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		var body recordConsentRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			writeDemandError(w, r, http.StatusBadRequest, "CONSENT_INVALID", "Согласие не удалось прочитать.", false, nil)
			return
		}
		id, err := persistentid.New()
		if err != nil {
			writeDemandError(w, r, http.StatusInternalServerError, "CONSENT_ID_FAILED", "Не удалось создать запись согласия.", true, nil)
			return
		}
		record, err := privacy.NewConsentRecord(privacy.ConsentRecord{
			ID: id,
			SubjectID: identityID,
			Purpose: strings.TrimSpace(body.Purpose),
			Scope: strings.TrimSpace(body.Scope),
			PolicyVersion: strings.TrimSpace(body.PolicyVersion),
			TextHashOrVersion: strings.TrimSpace(body.TextHashOrVersion),
			GrantedAt: now().UTC(),
			Source: strings.TrimSpace(body.Source),
			ProofMetadata: body.ProofMetadata,
		})
		if err != nil {
			writeDemandError(w, r, http.StatusBadRequest, "CONSENT_INVALID", "Согласие некорректно.", false, nil)
			return
		}
		persisted, idempotent, err := store.RecordConsent(record)
		if err != nil {
			if errors.Is(err, privacy.ErrInvalidConsent) {
				writeDemandError(w, r, http.StatusBadRequest, "CONSENT_INVALID", "Согласие некорректно.", false, nil)
				return
			}
			writeDemandError(w, r, http.StatusServiceUnavailable, "CONSENT_STORAGE_FAILED", "Не удалось сохранить согласие.", true, nil)
			return
		}
		status := http.StatusCreated
		if idempotent {
			status = http.StatusOK
		}
		writeJSON(w, status, persisted)
	})

	mux.HandleFunc("POST /v1/consents/{consentID}/revoke", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "CONSENT_STORE_UNAVAILABLE", "Хранилище согласий недоступно.", true, nil)
			return
		}
		identityID, ok := requiredClientSessionIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		record, idempotent, err := store.RevokeConsent(strings.TrimSpace(r.PathValue("consentID")), identityID, now().UTC())
		if err != nil {
			if errors.Is(err, privacy.ErrConsentNotFound) {
				writeDemandError(w, r, http.StatusNotFound, "CONSENT_NOT_FOUND", "Согласие не найдено.", false, nil)
				return
			}
			writeDemandError(w, r, http.StatusServiceUnavailable, "CONSENT_STORAGE_FAILED", "Не удалось отозвать согласие.", true, nil)
			return
		}
		status := http.StatusOK
		if idempotent {
			status = http.StatusOK
		}
		writeJSON(w, status, record)
	})
}
