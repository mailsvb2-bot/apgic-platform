package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/privacy"
)

type accountDeletionIntakeStore interface {
	SubmitAccountDeletionRequest(context.Context, string, string) (privacy.AccountDeletionReceipt, error)
	ReadAccountDeletionRequest(context.Context, string) (privacy.AccountDeletionReceipt, error)
}

func registerAccountDeletionIntake(mux *http.ServeMux, store accountDeletionIntakeStore, sessions *clientSessionManager, stepUp *stepUpManager, sessionErr error) {
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

	// Confirm ownership of the already-created durable request, never erasure.
	// Proof must originate from a verified second factor bound to this exact
	// signed client session. A fabricated flag in the request body is ignored.
	mux.HandleFunc("POST /v1/client/account-deletion-requests/reconfirm", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		if sessions == nil || stepUp == nil || sessionErr != nil || store == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DELETION_CONFIRMATION_UNAVAILABLE", "Проверка личности недоступна.", true, nil)
			return
		}
		owner, sessionRef, err := sessions.identityAndReferenceFromRequest(r)
		if err != nil || strings.TrimSpace(owner) == "" {
			writeDemandError(w, r, http.StatusUnauthorized, "CLIENT_SESSION_REQUIRED", "Недействительная сессия.", false, nil)
			return
		}
		if r.Header.Get("X-APGIC-Deletion-Confirmation") != "verified-step-up-v1" ||
			!strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			writeDemandError(w, r, http.StatusUnsupportedMediaType, "DELETION_CONFIRMATION_INVALID", "Требуется подтверждённый запрос.", false, nil)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 256)
		var input struct {
			Confirm bool `json:"confirm"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || !input.Confirm {
			writeDemandError(w, r, http.StatusBadRequest, "DELETION_CONFIRMATION_INVALID", "Требуется явное согласие.", false, nil)
			return
		}
		var extra any
		if !errors.Is(decoder.Decode(&extra), io.EOF) {
			writeDemandError(w, r, http.StatusBadRequest, "DELETION_CONFIRMATION_INVALID", "Требуется один объект.", false, nil)
			return
		}
		evidence := stepUp.evidenceFromRequest(r, owner, sessionRef)
		if evidence == nil || evidence.Method != "TOTP" || time.Since(evidence.IssuedAt) > stepUpTTL {
			writeDemandError(w, r, http.StatusPreconditionRequired, "DELETION_STEP_UP_REQUIRED", "Требуется действующее подтверждение второго фактора.", false, nil)
			return
		}
		factors, ok := store.(interface {
			FinanceMFAActiveAt(context.Context, string, time.Time) (bool, error)
		})
		if !ok {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DELETION_FACTOR_UNAVAILABLE", "Проверка второго фактора недоступна.", true, nil)
			return
		}
		active, err := factors.FinanceMFAActiveAt(r.Context(), owner, evidence.IssuedAt)
		if err != nil || !active {
			writeDemandError(w, r, http.StatusForbidden, "DELETION_STEP_UP_REVOKED", "Подтверждение личности отозвано или недействительно.", false, nil)
			return
		}
		confirm, ok := store.(interface {
			ReconfirmAccountDeletionRequest(context.Context, string, string, time.Time) (privacy.AccountDeletionReceipt, error)
		})
		if !ok {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DELETION_CONFIRMATION_UNAVAILABLE", "Завершение проверки недоступно.", true, nil)
			return
		}
		value, err := confirm.ReconfirmAccountDeletionRequest(r.Context(), owner, sessionRef, evidence.IssuedAt)
		if err != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DELETION_CONFIRMATION_UNAVAILABLE", "Состояние запроса не обновлено.", true, nil)
			return
		}
		writeJSON(w, http.StatusOK, value)
	})
	mux.HandleFunc("GET /v1/client/account-deletion-requests", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		if sessions == nil || sessionErr != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "CLIENT_SESSION_UNAVAILABLE", "Сессия клиента недоступна.", true, nil)
			return
		}
		identity, err := sessions.identityForDeletionReceipt(r)
		if err != nil {
			writeDemandError(w, r, http.StatusUnauthorized, "CLIENT_SESSION_REQUIRED", "Недействительная сессия.", false, nil)
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
