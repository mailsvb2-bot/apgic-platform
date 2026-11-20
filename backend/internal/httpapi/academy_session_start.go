package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// Author onboarding is an explicit entrypoint to the existing signed Identity.
// Never create a new identity from a course mutation or overwrite a valid one.
func registerAcademySessionStart(mux *http.ServeMux, sessions *clientSessionManager, sessionErr error) {
	mux.HandleFunc("POST /v1/academy/session/start", func(w http.ResponseWriter, r *http.Request) {
		if sessionErr != nil || sessions == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "ACADEMY_SESSION_UNAVAILABLE", "Подписанная сессия недоступна.", true, nil)
			return
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			writeDemandError(w, r, http.StatusForbidden, "ACADEMY_SESSION_ORIGIN_DENIED", "Непроверенный источник запроса.", false, nil)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || !strings.EqualFold(u.Host, r.Host) || (u.Scheme != "https" && !(u.Scheme == "http" && strings.HasPrefix(r.Host, "127.0.0.1:"))) {
				writeDemandError(w, r, http.StatusForbidden, "ACADEMY_SESSION_ORIGIN_DENIED", "Непроверенный источник запроса.", false, nil)
				return
			}
		}
		identity, cookie, err := sessions.identityForCreate(r)
		if err != nil {
			code := "CLIENT_SESSION_INVALID"
			if !errors.Is(err, ErrClientSessionInvalid) && !errors.Is(err, ErrClientSessionExpired) {
				code = "ACADEMY_SESSION_UNAVAILABLE"
			}
			writeDemandError(w, r, http.StatusUnauthorized, code, "Сессия недействительна. Подключите существующий аккаунт, чтобы не потерять курсы.", false, nil)
			return
		}
		if identity == "" {
			writeDemandError(w, r, http.StatusServiceUnavailable, "ACADEMY_SESSION_UNAVAILABLE", "Не удалось открыть сессию автора.", true, nil)
			return
		}
		if cookie != nil {
			http.SetCookie(w, cookie)
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, map[string]any{"session_ready": true, "new_identity": cookie != nil})
	})
}
