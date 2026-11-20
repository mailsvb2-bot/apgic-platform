package httpapi

import (
	"errors"
	"net/http"
)

// Author onboarding is an explicit entrypoint to the existing signed Identity.
// Never create a new identity from a course mutation or overwrite a valid one.
func registerAcademySessionStart(mux *http.ServeMux, store academyStore, sessions *clientSessionManager, sessionErr error) {
	mux.HandleFunc("POST /v1/academy/session/start", func(w http.ResponseWriter, r *http.Request) {
		if sessionErr != nil || sessions == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "ACADEMY_SESSION_UNAVAILABLE", "Подписанная сессия недоступна.", true, nil)
			return
		}
		// Reuse the verified first-party Origin policy used for device pairing.
		// Next.js proxies public requests to an internal upstream Host.
		if !accountPairOrigin(r) {
			writeDemandError(w, r, http.StatusForbidden, "ACADEMY_SESSION_ORIGIN_DENIED", "Непроверенный источник запроса.", false, nil)
			return
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
		// Persist the *same signed Identity* in canonical PostgreSQL before
		// issuing the cookie. A session without an identities row cannot own a
		// Commerce product: the owner FK trigger rejects Academy drafts.
		registrar, ok := store.(interface{ AcademyEnsureIdentity(string) error })
		if !ok {
			writeDemandError(w, r, http.StatusServiceUnavailable, "ACADEMY_SESSION_UNAVAILABLE",
				"Хранилище авторской сессии недоступно.", true, nil)
			return
		}
		if err := registrar.AcademyEnsureIdentity(identity); err != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "ACADEMY_SESSION_UNAVAILABLE",
				"Не удалось сохранить авторскую сессию. Повторите позже.", true, nil)
			return
		}
		if cookie != nil {
			http.SetCookie(w, cookie)
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, map[string]any{"session_ready": true, "new_identity": cookie != nil})
	})
}
