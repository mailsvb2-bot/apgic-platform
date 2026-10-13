package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/specialist"
)

// A verified client rating is tied to an actually completed live booking.
// No caller can attest completion, pick a specialist or mark it approved.
func registerSpecialistFeedback(mux *http.ServeMux, store specialist.Store,
	sessions *clientSessionManager, sessionConfigErr error) {
	feedback, _ := store.(specialist.FeedbackStore)
	mux.HandleFunc("POST /v1/specialist-feedback", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		clientID, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		if feedback == nil {
			writeSpecialistError(w, r, http.StatusServiceUnavailable, "SPECIALIST_FEEDBACK_UNAVAILABLE", "Оценки консультаций сейчас недоступны.")
			return
		}
		var body struct {
			BookingID string `json:"booking_id"`
			Rating    int    `json:"rating"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			writeSpecialistError(w, r, http.StatusBadRequest, "SPECIALIST_FEEDBACK_INVALID", "Укажите завершённую запись и оценку от 1 до 5.")
			return
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			writeSpecialistError(w, r, http.StatusBadRequest, "SPECIALIST_FEEDBACK_INVALID", "Ожидается только один объект оценки.")
			return
		}
		result, err := feedback.SubmitVerifiedFeedback(clientID, body.BookingID, body.Rating)
		switch {
		case errors.Is(err, specialist.ErrFeedbackInvalid):
			writeSpecialistError(w, r, http.StatusBadRequest, "SPECIALIST_FEEDBACK_INVALID", "Проверьте идентификатор записи и оценку.")
		case errors.Is(err, specialist.ErrFeedbackNotEligible):
			writeSpecialistError(w, r, http.StatusConflict, "SPECIALIST_FEEDBACK_NOT_ELIGIBLE", "Оценить можно только собственную завершённую консультацию.")
		case errors.Is(err, specialist.ErrFeedbackDuplicate):
			writeSpecialistError(w, r, http.StatusConflict, "SPECIALIST_FEEDBACK_ALREADY_SUBMITTED", "Оценка этой консультации уже зарегистрирована.")
		case err != nil:
			writeSpecialistError(w, r, http.StatusServiceUnavailable, "SPECIALIST_FEEDBACK_UNAVAILABLE", "Не удалось сохранить оценку.")
		default:
			writeJSON(w, http.StatusCreated, result)
		}
	})
}
