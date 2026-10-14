package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/connector"
)

type ConsultationJourneyStore interface {
	ReadConsultationReady(context.Context, string, string) (*connector.ReadySnapshot, error)
	ConfirmConsultationReady(context.Context, string, string, time.Time) (*connector.ReadySnapshot, error)
	ReadConsultationOutcome(context.Context, string, string) (*connector.OutcomeReport, error)
	ReadConsultationTimeline(context.Context, string, string) (*connector.ConsultationTimeline, error)
	RecordConsultationOutcome(context.Context, string, string, string, int, int, time.Time) (*connector.OutcomeReport, error)
}

var consultationUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func registerConsultationJourney(mux *http.ServeMux, store ConsultationJourneyStore, sessions *clientSessionManager, configErr error, now func() time.Time) {
	identityFor := func(w http.ResponseWriter, r *http.Request) (string, bool) {
		if !consultationUUID.MatchString(r.PathValue("bookingID")) {
			writeDemandError(w, r, http.StatusBadRequest, "CONSULT_BOOKING_ID_INVALID", "Некорректный идентификатор консультации.", false, nil)
			return "", false
		}
		return trustedClientIdentity(w, r, sessions, configErr, "")
	}
	available := func(w http.ResponseWriter, r *http.Request) bool {
		if store == nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "CONSULT_JOURNEY_UNAVAILABLE", "Маршрут консультации временно недоступен.", true, nil)
			return false
		}
		return true
	}
	mutation := func(w http.ResponseWriter, r *http.Request) bool {
		// A non-simple request header prevents cross-site HTML form posts from
		// silently mutating a signed-cookie session.
		if r.Header.Get("X-APGIC-Request") != "consultation-v1" {
			writeDemandError(w, r, http.StatusForbidden, "CONSULT_REQUEST_ORIGIN_UNVERIFIED", "Запрос не прошёл проверку происхождения.", false, nil)
			return false
		}
		return true
	}
	mux.HandleFunc("GET /v1/consultations/{bookingID}/ready", func(w http.ResponseWriter, r *http.Request) {
		if !available(w, r) {
			return
		}
		identity, ok := identityFor(w, r)
		if !ok {
			return
		}
		v, err := store.ReadConsultationReady(r.Context(), r.PathValue("bookingID"), identity)
		if err != nil {
			consultationStorageError(w, r)
			return
		}
		if v == nil {
			consultationHidden(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("POST /v1/consultations/{bookingID}/ready", func(w http.ResponseWriter, r *http.Request) {
		if !available(w, r) || !mutation(w, r) {
			return
		}
		identity, ok := identityFor(w, r)
		if !ok {
			return
		}
		v, err := store.ConfirmConsultationReady(r.Context(), r.PathValue("bookingID"), identity, now().UTC())
		if errors.Is(err, connector.ErrConsultationNotReady) {
			writeDemandError(w, r, http.StatusConflict, "CONSULT_READY_WINDOW_CLOSED", "Отметка готовности доступна только для подтверждённой записи в разрешённое время.", false, nil)
			return
		}
		if err != nil {
			consultationStorageError(w, r)
			return
		}
		if v == nil {
			consultationHidden(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("GET /v1/consultations/{bookingID}/timeline", func(w http.ResponseWriter, r *http.Request) {
		if !available(w, r) {
			return
		}
		identity, ok := identityFor(w, r)
		if !ok {
			return
		}
		timeline, err := store.ReadConsultationTimeline(r.Context(), r.PathValue("bookingID"), identity)
		if err != nil {
			consultationStorageError(w, r)
			return
		}
		if timeline == nil {
			consultationHidden(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, timeline)
	})
	mux.HandleFunc("GET /v1/consultations/{bookingID}/outcome", func(w http.ResponseWriter, r *http.Request) {
		if !available(w, r) {
			return
		}
		identity, ok := identityFor(w, r)
		if !ok {
			return
		}
		v, err := store.ReadConsultationOutcome(r.Context(), r.PathValue("bookingID"), identity)
		if err != nil {
			consultationStorageError(w, r)
			return
		}
		if v == nil {
			consultationHidden(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("POST /v1/consultations/{bookingID}/outcome", func(w http.ResponseWriter, r *http.Request) {
		if !available(w, r) || !mutation(w, r) {
			return
		}
		identity, ok := identityFor(w, r)
		if !ok {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 2048)
		var req struct {
			Progress          int    `json:"progress"`
			MatchSatisfaction int    `json:"match_satisfaction"`
			IdempotencyKey    string `json:"idempotency_key"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil || strings.TrimSpace(req.IdempotencyKey) == "" {
			writeDemandError(w, r, http.StatusBadRequest, "CONSULT_OUTCOME_INVALID", "Некорректный отчёт клиента.", false, nil)
			return
		}
		v, err := store.RecordConsultationOutcome(r.Context(), r.PathValue("bookingID"), identity, req.IdempotencyKey, req.Progress, req.MatchSatisfaction, now().UTC())
		switch {
		case errors.Is(err, connector.ErrConsultationOutcomeInvalid):
			writeDemandError(w, r, http.StatusBadRequest, "CONSULT_OUTCOME_INVALID", "Недопустимая оценка результата.", false, nil)
			return
		case errors.Is(err, connector.ErrConsultationOutcomeNotReady):
			writeDemandError(w, r, http.StatusConflict, "CONSULT_COMPLETION_UNVERIFIED", "Завершение консультации ещё не подтверждено провайдером.", false, nil)
			return
		case errors.Is(err, connector.ErrConsultationOutcomeConflict):
			writeDemandError(w, r, http.StatusConflict, "CONSULT_OUTCOME_IDEMPOTENCY_CONFLICT", "Повторный ключ нельзя использовать для другого результата.", false, nil)
			return
		case err != nil:
			consultationStorageError(w, r)
			return
		}
		if v == nil {
			consultationHidden(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, v)
	})
}
func consultationHidden(w http.ResponseWriter, r *http.Request) {
	writeDemandError(w, r, http.StatusNotFound, "CONSULT_NOT_FOUND", "Консультация не найдена или доступ к ней отсутствует.", false, nil)
}
func consultationStorageError(w http.ResponseWriter, r *http.Request) {
	writeDemandError(w, r, http.StatusServiceUnavailable, "CONSULT_STORE_UNAVAILABLE", "Данные консультации временно недоступны.", true, nil)
}
