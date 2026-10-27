package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/privacy"
)

type accountDeletionIntakeStore interface {
	SubmitAccountDeletionRequest(context.Context, string, string) (privacy.AccountDeletionReceipt, error)
	ReadAccountDeletionRequest(context.Context, string) (privacy.AccountDeletionReceipt, error)
	IsAccountAccessRevoked(context.Context, string) (bool, error)
	ReconfirmAccountDeletion(context.Context, string, string, string, time.Time) (privacy.AccountDeletionReceipt, error)
}

func registerAccountDeletionIntake(mux *http.ServeMux, store accountDeletionIntakeStore, sessions *clientSessionManager, sessionErr error, stepUp *stepUpManager) {
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

	mux.HandleFunc("POST /v1/client/account-deletion-requests/reconfirm", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		identity, ok := requiredClientSessionIdentity(w, r, sessions, sessionErr)
		if !ok {
			return
		}
		if store == nil || stepUp == nil || sessions == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DELETION_RECONFIRM_UNAVAILABLE", "Подтверждение личности недоступно.", true, nil)
			return
		}
		owner, sessionRef, err := sessions.identityAndReferenceFromRequest(r)
		if err != nil || owner != identity {
			writeDemandError(w, r, http.StatusUnauthorized, "CLIENT_SESSION_REQUIRED", "Требуется действующая сессия.", false, nil)
			return
		}
		evidence := stepUp.evidenceFromRequest(r, identity, sessionRef)
		if evidence == nil || (evidence.Method != "WEBAUTHN" && evidence.Method != "TOTP") {
			writeDemandError(w, r, http.StatusForbidden, "DELETION_FRESH_STEP_UP_REQUIRED", "Нужно повторное подтверждение личности.", false, nil)
			return
		}
		// A TOTP assertion is not enough by itself: the signing cookie can
		// outlive factor revocation. Check the factor's canonical active state.
		if evidence.Method == "TOTP" {
			checker, supported := store.(interface {
				FinanceMFAActiveAt(context.Context, string, time.Time) (bool, error)
			})
			if !supported {
				writeDemandError(w, r, http.StatusServiceUnavailable, "DELETION_FACTOR_STORE_UNAVAILABLE", "Проверка фактора недоступна.", true, nil)
				return
			}
			active, factorErr := checker.FinanceMFAActiveAt(r.Context(), identity, evidence.IssuedAt)
			if factorErr != nil {
				writeDemandError(w, r, http.StatusServiceUnavailable, "DELETION_FACTOR_STORE_UNAVAILABLE", "Проверка фактора недоступна.", true, nil)
				return
			}
			if !active {
				writeDemandError(w, r, http.StatusForbidden, "DELETION_VERIFIABLE_FACTOR_REQUIRED", "Необходимо подтверждение действующего фактора.", false, nil)
				return
			}
		}
		value, err := store.ReconfirmAccountDeletion(r.Context(), identity, sessionRef, evidence.Method, evidence.IssuedAt)
		if err != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DELETION_RECONFIRM_UNAVAILABLE", "Не удалось сохранить подтверждение.", true, nil)
			return
		}
		writeJSON(w, http.StatusOK, value)
	})
	mux.HandleFunc("GET /v1/client/account-deletion-requests", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		// This read is the sole narrowly scoped exception to session revocation:
		// an erased client can still see the nonpersonal deletion receipt until
		// their signed cookie expires. No other account data is exposed.
		if sessions == nil || sessionErr != nil {
			writeDemandError(w, r, http.StatusUnauthorized, "CLIENT_SESSION_REQUIRED", "Требуется сессия клиента.", false, nil)
			return
		}
		cookie, cookieErr := r.Cookie(clientSessionCookieName)
		if cookieErr != nil {
			writeDemandError(w, r, http.StatusUnauthorized, "CLIENT_SESSION_REQUIRED", "Требуется сессия клиента.", false, nil)
			return
		}
		identity, parseErr := sessions.parse(cookie.Value)
		if parseErr != nil {
			writeDemandError(w, r, http.StatusUnauthorized, "CLIENT_SESSION_REQUIRED", "Требуется сессия клиента.", false, nil)
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
