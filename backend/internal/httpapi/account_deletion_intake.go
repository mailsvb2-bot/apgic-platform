package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/privacy"
)

type accountDeletionIntakeStore interface {
	SubmitAccountDeletionRequest(context.Context, string, string) (privacy.AccountDeletionReceipt, error)
	ReadAccountDeletionRequest(context.Context, string) (privacy.AccountDeletionReceipt, error)
}

func registerAccountDeletionIntake(mux *http.ServeMux, store accountDeletionIntakeStore, sessions *clientSessionManager, sessionErr error) {
	mux.HandleFunc("POST /v1/client/account-deletion-requests", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		identity, ok := requiredClientSessionIdentity(w, r, sessions, sessionErr)
		if !ok {
			return
		}
		if store == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DELETION_INTAKE_UNAVAILABLE", "Регистрация запроса временно недоступна.", true, nil)
			return
		}
		var body struct {
			Source string `json:"source"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
		dec.DisallowUnknownFields()
		if dec.Decode(&body) != nil {
			writeDemandError(w, r, http.StatusBadRequest, "DELETION_SOURCE_INVALID", "Источник запроса недопустим.", false, nil)
			return
		}
		var extra any
		if !errors.Is(dec.Decode(&extra), io.EOF) {
			writeDemandError(w, r, http.StatusBadRequest, "DELETION_SOURCE_INVALID", "Требуется один объект.", false, nil)
			return
		}
		if body.Source != "WEB" && body.Source != "IOS" && body.Source != "ANDROID" {
			writeDemandError(w, r, http.StatusBadRequest, "DELETION_SOURCE_INVALID", "Источник запроса недопустим.", false, nil)
			return
		}
		value, err := store.SubmitAccountDeletionRequest(r.Context(), identity, body.Source)
		if err != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DELETION_INTAKE_UNAVAILABLE", "Регистрация запроса временно недоступна.", true, nil)
			return
		}
		status := http.StatusAccepted
		if value.Idempotent {
			status = http.StatusOK
		}
		writeJSON(w, status, value)
	})
	mux.HandleFunc("GET /v1/client/account-deletion-requests", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		identity, ok := requiredClientSessionIdentity(w, r, sessions, sessionErr)
		if !ok {
			return
		}
		if store == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DELETION_INTAKE_UNAVAILABLE", "Статус запроса временно недоступен.", true, nil)
			return
		}
		value, err := store.ReadAccountDeletionRequest(r.Context(), identity)
		if errors.Is(err, sql.ErrNoRows) {
			writeDemandError(w, r, http.StatusNotFound, "DELETION_REQUEST_NOT_FOUND", "Запрос ещё не зарегистрирован.", false, nil)
			return
		}
		if err != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DELETION_INTAKE_UNAVAILABLE", "Статус запроса временно недоступен.", true, nil)
			return
		}
		writeJSON(w, http.StatusOK, value)
	})
}
