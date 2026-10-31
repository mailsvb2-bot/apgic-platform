package httpapi

import (
	"net/http"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type academyNotificationPreferencesStore interface {
	AcademyNotificationPreferences(string) (academy.NotificationPreferences, error)
	AcademySaveNotificationPreferences(string, academy.NotificationPreferences) (academy.NotificationPreferences, error)
}

func registerAcademyNotificationPreferences(mux *http.ServeMux, store academyStore, sessions *clientSessionManager, sessionConfigErr error) {
	mux.HandleFunc("GET /v1/academy/me/notifications", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := academyIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		owner, ok := store.(academyNotificationPreferencesStore)
		if !ok {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		value, err := owner.AcademyNotificationPreferences(actor)
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, value)
	})
	mux.HandleFunc("POST /v1/academy/me/notifications", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := academyIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		owner, ok := store.(academyNotificationPreferencesStore)
		if !ok {
			academyHTTPError(w, r, academy.ErrUnavailable)
			return
		}
		var input academy.NotificationPreferences
		if !academyRead(w, r, &input) {
			return
		}
		value, err := owner.AcademySaveNotificationPreferences(actor, input)
		if err != nil {
			academyHTTPError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, value)
	})
}
