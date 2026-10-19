package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// FinanceMFAStore is the canonical verified-factor boundary; the API cannot
// self-issue step-up without a successful DB-serialized TOTP verification.
type FinanceMFAStore interface {
	VerifyFinanceMFA(context.Context, string, string, string, [32]byte, time.Time) (bool, error)
}

func registerFinanceMFA(mux *http.ServeMux, store FinanceMFAStore, sessions *clientSessionManager, manager *stepUpManager, key [32]byte, sessionErr error, now func() time.Time) {
	mux.HandleFunc("POST /v1/finance/mfa/verify", func(w http.ResponseWriter, r *http.Request) {
		var zero [32]byte
		if store == nil || sessions == nil || manager == nil || sessionErr != nil || key == zero {
			writeDemandError(w, r, http.StatusServiceUnavailable, "MFA_UNAVAILABLE", "Подтверждение личности сейчас недоступно.", true, nil)
			return
		}
		// Same-site cookies alone are insufficient CSRF protection on a financial
		// endpoint: accept only JSON with a non-simple custom header.
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") || r.Header.Get("X-APGIC-MFA") != "totp-v1" {
			writeDemandError(w, r, http.StatusUnsupportedMediaType, "MFA_REQUEST_INVALID", "Неверный формат запроса.", false, nil)
			return
		}
		identity, sessionRef, err := sessions.identityAndReferenceFromRequest(r)
		if err != nil || identity == "" {
			writeDemandError(w, r, http.StatusUnauthorized, "AUTH_SESSION_INVALID", "Недействительная сессия.", false, nil)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 256)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var input struct {
			Code string `json:"code"`
		}
		if err = decoder.Decode(&input); err != nil || len(input.Code) != 6 {
			writeDemandError(w, r, http.StatusBadRequest, "MFA_REQUEST_INVALID", "Введите шестизначный код.", false, nil)
			return
		}
		var trailing any
		if decoder.Decode(&trailing) != io.EOF {
			writeDemandError(w, r, http.StatusBadRequest, "MFA_REQUEST_INVALID", "Запрос должен содержать один объект.", false, nil)
			return
		}
		verified, err := store.VerifyFinanceMFA(r.Context(), identity, sessionRef, input.Code, key, now().UTC())
		if err != nil || !verified {
			// Never reveal enrollment status or valid operator grants.
			writeDemandError(w, r, http.StatusForbidden, "MFA_VERIFICATION_FAILED", "Код отклонён или временно недоступен.", false, nil)
			return
		}
		cookie, err := manager.issue(identity, sessionRef, "TOTP", now().UTC())
		if err != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "MFA_UNAVAILABLE", "Подтверждение личности временно недоступно.", true, nil)
			return
		}
		http.SetCookie(w, cookie)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNoContent)
	})
}
