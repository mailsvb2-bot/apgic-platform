package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/clientpersonal"
)

type clientPersonalStore interface {
	ReadClientPersonal(context.Context, string, int, *clientpersonal.HistoryCursor, time.Time) (clientpersonal.View, error)
	UpdateClientPersonalPreferences(context.Context, string, clientpersonal.Preferences) (clientpersonal.Preferences, error)
}

func registerClientPersonal(mux *http.ServeMux, store clientPersonalStore, sessions *clientSessionManager, sessionConfigErr error, now func() time.Time) {
	mux.HandleFunc("GET /v1/client/personal", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		identity, ok := requiredClientSessionIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		if store == nil {
			writeClientPersonalUnavailable(w, r)
			return
		}
		if r.URL.Query().Has("identity_id") {
			writeDemandError(w, r, http.StatusBadRequest, "PERSONAL_IDENTITY_NOT_ACCEPTED", "Идентификатор клиента задаётся сессией.", false, nil)
			return
		}
		limit := 20
		if raw := r.URL.Query().Get("limit"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > clientpersonal.MaxHistoryLimit {
				writeDemandError(w, r, http.StatusBadRequest, "PERSONAL_LIMIT_INVALID", "Размер страницы истории недопустим.", false, nil)
				return
			}
			limit = n
		}
		cursor, err := clientpersonal.DecodeCursor(r.URL.Query().Get("history_cursor"))
		if err != nil {
			writeDemandError(w, r, http.StatusBadRequest, "PERSONAL_CURSOR_INVALID", "Курсор истории недействителен.", false, nil)
			return
		}
		view, err := store.ReadClientPersonal(r.Context(), identity, limit, cursor, now())
		if errors.Is(err, clientpersonal.ErrIdentityNotFound) {
			writeDemandError(w, r, http.StatusNotFound, "PERSONAL_IDENTITY_NOT_FOUND", "Профиль не найден.", false, nil)
			return
		}
		if err != nil {
			writeClientPersonalUnavailable(w, r)
			return
		}
		writeJSON(w, http.StatusOK, view)
	})
	mux.HandleFunc("PUT /v1/client/personal/preferences", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		identity, ok := requiredClientSessionIdentity(w, r, sessions, sessionConfigErr)
		if !ok {
			return
		}
		if store == nil {
			writeClientPersonalUnavailable(w, r)
			return
		}
		var p clientpersonal.Preferences
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&p); err != nil {
			writeDemandError(w, r, http.StatusBadRequest, "PERSONAL_PREFERENCES_INVALID", "Настройки не распознаны.", false, nil)
			return
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			writeDemandError(w, r, http.StatusBadRequest, "PERSONAL_PREFERENCES_INVALID", "Ожидался один объект настроек.", false, nil)
			return
		}
		p.DisplayName = strings.TrimSpace(p.DisplayName)
		if err := clientpersonal.ValidatePreferences(p); err != nil {
			writeDemandError(w, r, http.StatusBadRequest, "PERSONAL_PREFERENCES_INVALID", "Проверьте имя, язык и часовой пояс.", false, nil)
			return
		}
		updated, err := store.UpdateClientPersonalPreferences(r.Context(), identity, p)
		if err != nil {
			writeClientPersonalUnavailable(w, r)
			return
		}
		writeJSON(w, http.StatusOK, updated)
	})
}
func writeClientPersonalUnavailable(w http.ResponseWriter, r *http.Request) {
	writeDemandError(w, r, http.StatusServiceUnavailable, "PERSONAL_STORE_UNAVAILABLE", "Личный кабинет временно недоступен. Повторите попытку.", true, nil)
}
