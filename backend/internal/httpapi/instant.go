package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/instant"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

// Specialist payloads intentionally exclude client and HelpIntent identifiers.
// The dispatch ID is an opaque capability scoped by the trusted specialist
// session on each subsequent command; it never authorizes by itself.
type dutySpecialistProposal struct {
	ID         string     `json:"id"`
	State      string     `json:"state"`
	TopicID    string     `json:"topic_id,omitempty"`
	PriceMinor int64      `json:"price_minor,omitempty"`
	Currency   string     `json:"currency,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	NextAction string     `json:"next_action"`
	RoomReady  bool       `json:"room_ready"`
}

func dutySpecialistView(d instant.Dispatch) dutySpecialistProposal {
	return dutySpecialistProposal{
		ID: d.ID, State: d.State, TopicID: d.TopicID, PriceMinor: d.PriceMinor,
		Currency: d.Currency, ExpiresAt: d.ExpiresAt, NextAction: d.NextAction, RoomReady: false,
	}
}

// Duty/Now is a distinct command contract from scheduled slot booking.
// An ACCEPTED offer is not a paid booking and cannot issue room credentials.
func registerInstant(mux *http.ServeMux, store instant.Store,
	sessions *clientSessionManager, sessionConfigErr error, enabled bool) {
	authorize := func(w http.ResponseWriter, r *http.Request) (string, bool) {
		w.Header().Set("Cache-Control", "no-store")
		if !enabled {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DUTY_NOT_ENABLED", "Консультации сейчас пока недоступны.", false, nil)
			return "", false
		}
		if store == nil || sessions == nil || sessionConfigErr != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "DUTY_UNAVAILABLE", "Сервис дежурства недоступен.", true, nil)
			return "", false
		}
		return trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
	}
	mux.HandleFunc("GET /v1/instant/request-key", func(w http.ResponseWriter, r *http.Request) {
		_, ok := authorize(w, r)
		if !ok {
			return
		}
		id, err := persistentid.New()
		if err != nil {
			writeInstantError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"request_key": id})
	})
	mux.HandleFunc("GET /v1/instant/presence", func(w http.ResponseWriter, r *http.Request) {
		identity, ok := authorize(w, r)
		if !ok {
			return
		}
		presence, err := store.GetPresence(identity)
		if err != nil {
			writeInstantError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, presence)
	})
	mux.HandleFunc("PUT /v1/instant/presence", func(w http.ResponseWriter, r *http.Request) {
		identity, ok := authorize(w, r)
		if !ok {
			return
		}
		var body struct {
			OfferID   string `json:"offer_id"`
			Available *bool  `json:"available"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if err := decodeInstantCommand(r, &body); err != nil || body.Available == nil {
			writeInstantError(w, r, instant.ErrInvalid)
			return
		}
		presence, err := store.SetPresence(identity, body.OfferID, *body.Available)
		if err != nil {
			writeInstantError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, presence)
	})
	mux.HandleFunc("GET /v1/instant/inbox", func(w http.ResponseWriter, r *http.Request) {
		identity, ok := authorize(w, r)
		if !ok {
			return
		}
		items, err := store.Inbox(identity)
		if err != nil {
			writeInstantError(w, r, err)
			return
		}
		projections := make([]dutySpecialistProposal, 0, len(items))
		for _, item := range items {
			projections = append(projections, dutySpecialistView(item))
		}
		writeJSON(w, http.StatusOK, map[string]any{"dispatches": projections})
	})
	mux.HandleFunc("POST /v1/instant/dispatches", func(w http.ResponseWriter, r *http.Request) {
		identity, ok := authorize(w, r)
		if !ok {
			return
		}
		var body struct {
			HelpIntentID string `json:"help_intent_id"`
			RequestKey   string `json:"request_key"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if err := decodeInstantCommand(r, &body); err != nil {
			writeInstantError(w, r, instant.ErrInvalid)
			return
		}
		dispatch, err := store.Request(identity, body.HelpIntentID, body.RequestKey)
		if err != nil {
			writeInstantError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, dispatch)
	})
	mux.HandleFunc("GET /v1/instant/active", func(w http.ResponseWriter, r *http.Request) {
		identity, ok := authorize(w, r)
		if !ok {
			return
		}
		dispatch, err := store.ActiveDispatch(identity)
		if err != nil {
			writeInstantError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, dispatch)
	})
	mux.HandleFunc("GET /v1/instant/dispatches/{id}", func(w http.ResponseWriter, r *http.Request) {
		identity, ok := authorize(w, r)
		if !ok {
			return
		}
		dispatch, err := store.GetDispatch(r.PathValue("id"), identity)
		if err != nil {
			writeInstantError(w, r, err)
			return
		}
		if dispatch.ClientIdentityID != identity {
			writeJSON(w, http.StatusOK, dutySpecialistView(dispatch))
		} else {
			writeJSON(w, http.StatusOK, dispatch)
		}
	})
	mux.HandleFunc("POST /v1/instant/dispatches/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		identity, ok := authorize(w, r)
		if !ok {
			return
		}
		dispatch, err := store.Cancel(r.PathValue("id"), identity)
		if err != nil {
			writeInstantError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, dispatch)
	})
	mux.HandleFunc("POST /v1/instant/dispatches/{id}/answer", func(w http.ResponseWriter, r *http.Request) {
		identity, ok := authorize(w, r)
		if !ok {
			return
		}
		var body struct {
			Accept *bool `json:"accept"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if err := decodeInstantCommand(r, &body); err != nil || body.Accept == nil {
			writeInstantError(w, r, instant.ErrInvalid)
			return
		}
		dispatch, err := store.Answer(r.PathValue("id"), identity, *body.Accept)
		if err != nil {
			writeInstantError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, dutySpecialistView(dispatch))
	})
}

func writeInstantError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, instant.ErrInvalid):
		writeDemandError(w, r, http.StatusBadRequest, "DUTY_REQUEST_INVALID", "Некорректный идентификатор или команда.", false, nil)
	case errors.Is(err, instant.ErrNotFound):
		writeDemandError(w, r, http.StatusNotFound, "DUTY_NOT_FOUND", "Предложение не найдено или недоступно вам.", false, nil)
	case errors.Is(err, instant.ErrNotEligible):
		writeDemandError(w, r, http.StatusConflict, "DUTY_NOT_ELIGIBLE", "Требуется подтверждённый запрос и проверенный доступный специалист.", false, []string{"QUALIFICATION_REQUIRED"})
	case errors.Is(err, instant.ErrConflict):
		writeDemandError(w, r, http.StatusConflict, "DUTY_STATE_CONFLICT", "Предложение занято, просрочено или уже обработано.", false, nil)
	default:
		writeDemandError(w, r, http.StatusServiceUnavailable, "DUTY_STORAGE_UNAVAILABLE", "Не удалось обновить дежурство. Повторите попытку.", true, nil)
	}
}

// decodeInstantCommand rejects unknown fields and trailing JSON commands.
// A single signed request must never smuggle an additional mutation payload.
func decodeInstantCommand(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return instant.ErrInvalid
		}
		return err
	}
	return nil
}
