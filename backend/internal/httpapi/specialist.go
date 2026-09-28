package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/specialist"
)

type specialistProfileRequest struct {
	DisplayName    string `json:"display_name"`
	ProfessionCode string `json:"profession_code"`
}

type specialistCapabilityRequest struct {
	TopicID string `json:"topic_id"`
}

type specialistEvidenceRequest struct {
	TopicID   string `json:"topic_id"`
	Kind      string `json:"kind"`
	Reference string `json:"reference"`
}

func registerSpecialist(
	mux *http.ServeMux,
	store specialist.Store,
	sessions *clientSessionManager,
	sessionConfigErr error,
) {
	mux.HandleFunc("PUT /v1/specialist/profile", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeSpecialistError(w, r, http.StatusServiceUnavailable, "SPECIALIST_STORE_UNAVAILABLE", "Профессиональный профиль временно недоступен.")
			return
		}
		var body specialistProfileRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeSpecialistError(w, r, http.StatusBadRequest, "SPECIALIST_PROFILE_INVALID", "Профиль не удалось прочитать.")
			return
		}
		if sessionConfigErr != nil || sessions == nil {
			writeSpecialistError(w, r, http.StatusServiceUnavailable, "CLIENT_SESSION_UNAVAILABLE", "Сессия пользователя недоступна.")
			return
		}
		identityID, cookie, err := sessions.identityForCreate(r)
		if err != nil {
			writeSpecialistError(w, r, http.StatusUnauthorized, "CLIENT_SESSION_INVALID", "Сессия пользователя недействительна.")
			return
		}
		profile, err := store.UpsertProfile(identityID, body.DisplayName, body.ProfessionCode)
		if err != nil {
			writeSpecialistFailure(w, r, err)
			return
		}
		if cookie != nil {
			http.SetCookie(w, cookie)
		}
		writeJSON(w, http.StatusOK, profile)
	})

	mux.HandleFunc("GET /v1/specialist/profile", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeSpecialistError(w, r, http.StatusServiceUnavailable, "SPECIALIST_STORE_UNAVAILABLE", "Профессиональный профиль временно недоступен.")
			return
		}
		identityID, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		profile, err := store.Profile(identityID)
		if err != nil {
			writeSpecialistFailure(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, profile)
	})

	mux.HandleFunc("POST /v1/specialist/capabilities", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeSpecialistError(w, r, http.StatusServiceUnavailable, "SPECIALIST_STORE_UNAVAILABLE", "Профессиональный профиль временно недоступен.")
			return
		}
		var body specialistCapabilityRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeSpecialistError(w, r, http.StatusBadRequest, "SPECIALIST_CAPABILITY_INVALID", "Направление не удалось прочитать.")
			return
		}
		identityID, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		profile, err := store.DeclareCapability(identityID, body.TopicID)
		if err != nil {
			writeSpecialistFailure(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, profile)
	})

	mux.HandleFunc("POST /v1/specialist/evidence", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeSpecialistError(w, r, http.StatusServiceUnavailable, "SPECIALIST_STORE_UNAVAILABLE", "Профессиональный профиль временно недоступен.")
			return
		}
		var body specialistEvidenceRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeSpecialistError(w, r, http.StatusBadRequest, "SPECIALIST_EVIDENCE_INVALID", "Подтверждение не удалось прочитать.")
			return
		}
		identityID, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		profile, err := store.SubmitEvidence(identityID, body.TopicID, body.Kind, body.Reference)
		if err != nil {
			writeSpecialistFailure(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, profile)
	})

	mux.HandleFunc("POST /v1/specialist/publish", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeSpecialistError(w, r, http.StatusServiceUnavailable, "SPECIALIST_STORE_UNAVAILABLE", "Профессиональный профиль временно недоступен.")
			return
		}
		var body specialistCapabilityRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeSpecialistError(w, r, http.StatusBadRequest, "SPECIALIST_PUBLISH_INVALID", "Запрос публикации не удалось прочитать.")
			return
		}
		identityID, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		result, err := store.Publish(identityID, body.TopicID)
		if err != nil {
			writeSpecialistFailure(w, r, err)
			return
		}
		status := http.StatusOK
		if !result.Allowed {
			status = http.StatusConflict
		}
		writeJSON(w, status, result)
	})

	mux.HandleFunc("POST /v1/specialist/unpublish", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeSpecialistError(w, r, http.StatusServiceUnavailable, "SPECIALIST_STORE_UNAVAILABLE", "Профессиональный профиль временно недоступен.")
			return
		}
		var body specialistCapabilityRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeSpecialistError(w, r, http.StatusBadRequest, "SPECIALIST_PUBLISH_INVALID", "Запрос снятия с публикации не удалось прочитать.")
			return
		}
		identityID, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		result, err := store.Unpublish(identityID, body.TopicID)
		if err != nil {
			writeSpecialistFailure(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
}

func writeSpecialistFailure(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, specialist.ErrProfileNotFound):
		writeSpecialistError(w, r, http.StatusNotFound, "SPECIALIST_PROFILE_NOT_FOUND", "Профессиональный профиль ещё не создан.")
	case errors.Is(err, specialist.ErrCapabilityNotFound):
		writeSpecialistError(w, r, http.StatusNotFound, "SPECIALIST_CAPABILITY_NOT_FOUND", "Сначала добавьте направление работы.")
	case errors.Is(err, specialist.ErrProfileInvalid):
		writeSpecialistError(w, r, http.StatusBadRequest, "SPECIALIST_PROFILE_INVALID", "Заполните имя и профессию.")
	case errors.Is(err, specialist.ErrCapabilityInvalid):
		writeSpecialistError(w, r, http.StatusBadRequest, "SPECIALIST_CAPABILITY_INVALID", "Это направление пока не поддерживается.")
	case errors.Is(err, specialist.ErrEvidenceInvalid):
		writeSpecialistError(w, r, http.StatusBadRequest, "SPECIALIST_EVIDENCE_INVALID", "Укажите тип и ссылку или номер подтверждения.")
	default:
		writeSpecialistError(w, r, http.StatusInternalServerError, "SPECIALIST_INTERNAL_ERROR", "Профессиональный профиль не удалось обновить.")
	}
}

func writeSpecialistError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeDemandError(w, r, status, code, message, status >= http.StatusInternalServerError, nil)
}
