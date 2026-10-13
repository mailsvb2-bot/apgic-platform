package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/specialist"
)

// Directory reading is public; owner-editing is signed-session-only. No review
// or verification mutation is exposed on this API.
func registerPublicSpecialists(mux *http.ServeMux, store specialist.Store, sessions *clientSessionManager, sessionConfigErr error) {
	public, _ := store.(specialist.PublicDirectoryStore)
	mux.HandleFunc("GET /v1/specialists", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if public == nil {
			writeSpecialistError(w, r, http.StatusServiceUnavailable, "SPECIALIST_DIRECTORY_UNAVAILABLE", "Каталог специалистов пока недоступен.")
			return
		}
		topic := strings.TrimSpace(r.URL.Query().Get("topic"))
		if topic != "" {
			if _, err := specialist.NormalizeTopic(topic); err != nil {
				writeSpecialistError(w, r, http.StatusBadRequest, "SPECIALIST_TOPIC_INVALID", "Выберите поддерживаемое направление.")
				return
			}
		}
		limit := 10
		if raw := r.URL.Query().Get("limit"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 1 || parsed > 20 {
				writeSpecialistError(w, r, http.StatusBadRequest, "SPECIALIST_PAGE_INVALID", "Размер страницы должен быть от 1 до 20.")
				return
			}
			limit = parsed
		}
		offset := 0
		if raw := r.URL.Query().Get("offset"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 0 || parsed > 1000 {
				writeSpecialistError(w, r, http.StatusBadRequest, "SPECIALIST_PAGE_INVALID", "Недопустимая страница каталога.")
				return
			}
			offset = parsed
		}
		profiles, err := public.PublicProfiles(r.Context(), topic, limit+1, offset)
		if err != nil {
			writeSpecialistError(w, r, http.StatusServiceUnavailable, "SPECIALIST_DIRECTORY_UNAVAILABLE", "Не удалось загрузить проверенный каталог.")
			return
		}
		more := len(profiles) > limit
		if more {
			profiles = profiles[:limit]
		}
		writeJSON(w, http.StatusOK, map[string]any{"profiles": profiles, "topic": topic, "limit": limit, "offset": offset, "has_more": more, "policy_version": "qualification-v1", "sort": "DISPLAY_NAME_ASC"})
	})
	mux.HandleFunc("GET /v1/specialists/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if public == nil {
			writeSpecialistError(w, r, http.StatusServiceUnavailable, "SPECIALIST_DIRECTORY_UNAVAILABLE", "Каталог специалистов пока недоступен.")
			return
		}
		p, err := public.PublicProfile(r.Context(), r.PathValue("id"))
		if errors.Is(err, specialist.ErrPublicProfileNotFound) {
			writeSpecialistError(w, r, http.StatusNotFound, "SPECIALIST_NOT_PUBLIC", "Опубликованная анкета не найдена.")
			return
		}
		if err != nil {
			writeSpecialistError(w, r, http.StatusServiceUnavailable, "SPECIALIST_DIRECTORY_UNAVAILABLE", "Анкета пока недоступна.")
			return
		}
		writeJSON(w, http.StatusOK, p)
	})
	mux.HandleFunc("GET /v1/specialist/public-details", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if public == nil {
			writeSpecialistError(w, r, http.StatusServiceUnavailable, "SPECIALIST_DIRECTORY_UNAVAILABLE", "Анкета недоступна.")
			return
		}
		identity, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		details, err := public.PublicDetails(r.Context(), identity)
		if err != nil {
			writeSpecialistFailure(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, details)
	})
	mux.HandleFunc("PUT /v1/specialist/public-details", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if public == nil {
			writeSpecialistError(w, r, http.StatusServiceUnavailable, "SPECIALIST_DIRECTORY_UNAVAILABLE", "Анкета недоступна.")
			return
		}
		identity, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		var body specialist.PublicDetails
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			writeSpecialistError(w, r, http.StatusBadRequest, "SPECIALIST_DETAILS_INVALID", "Проверьте поля анкеты.")
			return
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			writeSpecialistError(w, r, http.StatusBadRequest, "SPECIALIST_DETAILS_INVALID", "Ожидается один объект анкеты.")
			return
		}
		details, err := public.SavePublicDetails(r.Context(), identity, body)
		if errors.Is(err, specialist.ErrPublicDetailsInvalid) {
			writeSpecialistError(w, r, http.StatusBadRequest, "SPECIALIST_DETAILS_INVALID", "Проверьте описание, опыт, языки и ограничения.")
			return
		}
		if err != nil {
			writeSpecialistFailure(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"details": details, "review_required": true, "notice": "Изменение профессиональных утверждений требует повторной проверки перед публикацией."})
	})
}
