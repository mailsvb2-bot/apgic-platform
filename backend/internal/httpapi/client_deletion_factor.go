package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/privacy"
)

type clientDeletionFactorStore interface {
	EnrollClientDeletionFactor(context.Context, string, [32]byte, time.Time) (string, privacy.DeletionFactorStatus, error)
	ClientDeletionFactorStatus(context.Context, string) (privacy.DeletionFactorStatus, error)
	VerifyClientDeletionFactor(context.Context, string, string, string, [32]byte, time.Time) (bool, error)
	DeletionFactorActiveAt(context.Context, string, time.Time) (bool, error)
}

// Independent possession factor for ordinary clients. It is purpose-bound,
// encrypted under a distinct key and inaccessible to finance endpoints.
func registerClientDeletionFactor(mux *http.ServeMux, store accountDeletionIntakeStore,
	sessions *clientSessionManager, stepUp *stepUpManager, key [32]byte,
	sessionErr error, now func() time.Time) {
	var zero [32]byte
	available := func(w http.ResponseWriter, r *http.Request) (clientDeletionFactorStore, bool) {
		factors, ok := store.(clientDeletionFactorStore)
		if !ok || sessions == nil || stepUp == nil || sessionErr != nil || key == zero {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DELETION_FACTOR_UNAVAILABLE",
				"Подтверждение удаления ещё не настроено.", true, nil)
			return nil, false
		}
		return factors, true
	}
	mux.HandleFunc("GET /v1/client/account-deletion-requests/factor", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		factors, ok := available(w, r)
		if !ok {
			return
		}
		identity, ok := requiredClientSessionIdentity(w, r, sessions, sessionErr)
		if !ok {
			return
		}
		status, err := factors.ClientDeletionFactorStatus(r.Context(), identity)
		if err != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DELETION_FACTOR_UNAVAILABLE",
				"Статус подтверждения недоступен.", true, nil)
			return
		}
		writeJSON(w, http.StatusOK, status)
	})
	mux.HandleFunc("POST /v1/client/account-deletion-requests/factor/enroll", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		factors, ok := available(w, r)
		if !ok {
			return
		}
		identity, ok := requiredClientSessionIdentity(w, r, sessions, sessionErr)
		if !ok {
			return
		}
		if r.Header.Get("X-APGIC-Deletion-Factor") != "enroll-v1" ||
			!strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			writeDemandError(w, r, http.StatusUnsupportedMediaType, "DELETION_FACTOR_REQUEST_INVALID",
				"Неверный формат подтверждения.", false, nil)
			return
		}
		var body struct {
			Acknowledge bool `json:"acknowledge"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256))
		dec.DisallowUnknownFields()
		if dec.Decode(&body) != nil || !body.Acknowledge {
			writeDemandError(w, r, http.StatusBadRequest, "DELETION_FACTOR_REQUEST_INVALID",
				"Нужно согласие на регистрацию проверочного кода.", false, nil)
			return
		}
		var trailing any
		if !errors.Is(dec.Decode(&trailing), io.EOF) {
			writeDemandError(w, r, http.StatusBadRequest, "DELETION_FACTOR_REQUEST_INVALID",
				"Требуется один объект.", false, nil)
			return
		}
		secret, status, err := factors.EnrollClientDeletionFactor(r.Context(), identity, key, now().UTC())
		if err != nil {
			writeDemandError(w, r, http.StatusConflict, "DELETION_FACTOR_ENROLLMENT_FAILED",
				"Фактор уже зарегистрирован или запрос недоступен.", false, nil)
			return
		}
		writeJSON(w, http.StatusCreated, struct {
			privacy.DeletionFactorStatus
			SecretBase32 string `json:"secret_base32"`
		}{DeletionFactorStatus: status, SecretBase32: secret})
	})
	mux.HandleFunc("POST /v1/client/account-deletion-requests/factor/verify", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		factors, ok := available(w, r)
		if !ok {
			return
		}
		identity, sessionRef, err := sessions.identityAndReferenceFromRequest(r)
		if err != nil || identity == "" {
			writeDemandError(w, r, http.StatusUnauthorized, "CLIENT_SESSION_REQUIRED",
				"Недействительная сессия.", false, nil)
			return
		}
		if r.Header.Get("X-APGIC-Deletion-Factor") != "verify-v1" ||
			!strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			writeDemandError(w, r, http.StatusUnsupportedMediaType, "DELETION_FACTOR_REQUEST_INVALID",
				"Неверный формат проверки.", false, nil)
			return
		}
		var body struct {
			Code string `json:"code"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256))
		dec.DisallowUnknownFields()
		if dec.Decode(&body) != nil || len(body.Code) != 6 {
			writeDemandError(w, r, http.StatusBadRequest, "DELETION_FACTOR_REQUEST_INVALID",
				"Требуется шестизначный код.", false, nil)
			return
		}
		var trailing any
		if !errors.Is(dec.Decode(&trailing), io.EOF) {
			writeDemandError(w, r, http.StatusBadRequest, "DELETION_FACTOR_REQUEST_INVALID",
				"Требуется один объект.", false, nil)
			return
		}
		verified, err := factors.VerifyClientDeletionFactor(r.Context(), identity, sessionRef,
			body.Code, key, now().UTC())
		if err != nil || !verified {
			writeDemandError(w, r, http.StatusForbidden, "DELETION_FACTOR_INVALID",
				"Код неверен, недоступен или срок ожидания ещё не закончился.", false, nil)
			return
		}
		cookie, err := stepUp.issue(identity, sessionRef, "DELETION_TOTP", now().UTC())
		if err != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DELETION_FACTOR_UNAVAILABLE",
				"Не удалось подтвердить владение.", true, nil)
			return
		}
		http.SetCookie(w, cookie)
		w.WriteHeader(http.StatusNoContent)
	})
}
