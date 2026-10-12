package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/connector"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/liveevents"
)

// The store owns SQL transactions; no in-memory event truth or payment stub.
type LiveEventStore interface {
	ListLiveEvents(context.Context) ([]liveevents.Event, error)
	ListOrganizationLiveEvents(context.Context, string, string) ([]liveevents.Event, error)
	ReadLiveEventAttendance(context.Context, string, string, string) (liveevents.AttendanceSummary, error)
	LiveEvent(context.Context, string, string) (liveevents.Event, error)
	CreateLiveEvent(context.Context, string, string, string, liveevents.DraftInput, time.Time) (liveevents.Event, error)
	ChangeLiveEvent(context.Context, string, string, liveevents.State, time.Time) (liveevents.Event, error)
	RegisterLiveEvent(context.Context, string, string, time.Time) (liveevents.Registration, error)
	MyLiveRegistration(context.Context, string, string) (*liveevents.Registration, error)
	CancelLiveRegistration(context.Context, string, string, time.Time) (liveevents.Registration, error)
	LiveEventAccess(context.Context, string, string, time.Time) (liveevents.Access, error)
	ApplyLiveProviderWebhook(context.Context, connector.WebhookEnvelope, connector.WebhookPublicKeyResolver) (connector.DeliveryDecision, error)
}

func decodeLiveJSON(w http.ResponseWriter, r *http.Request, out any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return liveevents.ErrInvalid
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return liveevents.ErrInvalid
	}
	return nil
}
func liveMutationAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	} // native clients need not send Origin
	u, err := url.Parse(origin)
	return err == nil && strings.EqualFold(u.Host, r.Host) &&
		(u.Scheme == "https" || (u.Scheme == "http" && strings.HasPrefix(r.Host, "127.0.0.1:")))
}
func liveError(w http.ResponseWriter, r *http.Request, err error) {
	code, status, desc := "LIVE_INTERNAL_ERROR", http.StatusInternalServerError, "Действие пока недоступно."
	switch {
	case errors.Is(err, liveevents.ErrInvalid):
		code, status, desc = "LIVE_EVENT_INVALID", 400, "Проверьте данные мероприятия."
	case errors.Is(err, liveevents.ErrForbidden):
		code, status, desc = "LIVE_EVENT_FORBIDDEN", 403, "Недостаточно прав."
	case errors.Is(err, liveevents.ErrNotFound):
		code, status, desc = "LIVE_EVENT_NOT_FOUND", 404, "Мероприятие не найдено."
	case errors.Is(err, liveevents.ErrConflict):
		code, status, desc = "LIVE_EVENT_CONFLICT", 409, "Состояние мероприятия изменилось или действие уже недоступно."
	case errors.Is(err, liveevents.ErrFull):
		code, status, desc = "LIVE_EVENT_FULL", 409, "Свободных мест больше нет."
	case errors.Is(err, liveevents.ErrPaymentRequired):
		code, status, desc = "LIVE_PAYMENT_NOT_CONFIGURED", 503, "Платная регистрация недоступна без подтверждённого платёжного провайдера."
	case errors.Is(err, liveevents.ErrUnavailable):
		code, status, desc = "LIVE_PROVIDER_UNAVAILABLE", 503, "Подтверждённый сервис групповой видеосвязи недоступен."
	}
	writeDemandError(w, r, status, code, desc, status >= 500, nil)
}
func registerLiveEvents(mux *http.ServeMux, store LiveEventStore, keys connector.WebhookPublicKeyResolver,
	sessions *clientSessionManager, sessionConfigErr error, now func() time.Time, media liveevents.RealMediaProvider) {
	mux.HandleFunc("GET /v1/organizations/{organizationID}/live-events/{eventID}/attendance", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			liveError(w, r, liveevents.ErrUnavailable)
			return
		}
		if sessions == nil || sessionConfigErr != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "CLIENT_SESSION_UNAVAILABLE", "Не настроены защищённые сессии.", false, nil)
			return
		}
		actor, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		summary, err := store.ReadLiveEventAttendance(r.Context(), actor, r.PathValue("organizationID"), r.PathValue("eventID"))
		if err != nil {
			liveError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, summary)
	})
	mux.HandleFunc("GET /v1/organizations/{organizationID}/live-events", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			liveError(w, r, liveevents.ErrUnavailable)
			return
		}
		if sessions == nil || sessionConfigErr != nil {
			writeDemandError(w, r, http.StatusServiceUnavailable, "CLIENT_SESSION_UNAVAILABLE", "Не настроены защищённые сессии.", false, nil)
			return
		}
		actor, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		events, err := store.ListOrganizationLiveEvents(r.Context(), actor, r.PathValue("organizationID"))
		if err != nil {
			liveError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]any{"events": events})
	})
	mux.HandleFunc("GET /v1/live-events", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			liveError(w, r, liveevents.ErrUnavailable)
			return
		}
		items, err := store.ListLiveEvents(r.Context())
		if err != nil {
			liveError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]any{"events": items})
	})
	mux.HandleFunc("GET /v1/live-events/{eventID}", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			liveError(w, r, liveevents.ErrUnavailable)
			return
		}
		actor := ""
		if sessions != nil && sessionConfigErr == nil {
			actor, _ = sessions.identityFromRequest(r)
		}
		event, err := store.LiveEvent(r.Context(), r.PathValue("eventID"), actor)
		if err != nil {
			liveError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, event)
	})
	// Owner-only publication: draft creation never silently publishes.
	mux.HandleFunc("POST /v1/organizations/{organizationID}/live-events", func(w http.ResponseWriter, r *http.Request) {
		if !liveMutationAllowed(r) {
			liveError(w, r, liveevents.ErrForbidden)
			return
		}
		if store == nil {
			liveError(w, r, liveevents.ErrUnavailable)
			return
		}
		if sessions == nil || sessionConfigErr != nil {
			writeDemandError(w, r, 503, "CLIENT_SESSION_UNAVAILABLE", "Не настроены защищённые сессии.", false, nil)
			return
		}
		actor, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		var input liveevents.DraftInput
		if err := decodeLiveJSON(w, r, &input); err != nil {
			liveError(w, r, liveevents.ErrInvalid)
			return
		}
		event, err := store.CreateLiveEvent(r.Context(), actor, r.PathValue("organizationID"), r.Header.Get("Idempotency-Key"), input, now().UTC())
		if err != nil {
			liveError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 201, event)
	})
	for _, transition := range []struct {
		path   string
		target liveevents.State
	}{
		{"publish", liveevents.Published}, {"start", liveevents.Live}, {"complete", liveevents.Completed}, {"cancel", liveevents.Cancelled},
	} {
		transition := transition
		mux.HandleFunc("POST /v1/live-events/{eventID}/"+transition.path, func(w http.ResponseWriter, r *http.Request) {
			if !liveMutationAllowed(r) {
				liveError(w, r, liveevents.ErrForbidden)
				return
			}
			if store == nil {
				liveError(w, r, liveevents.ErrUnavailable)
				return
			}
			if sessions == nil || sessionConfigErr != nil {
				writeDemandError(w, r, 503, "CLIENT_SESSION_UNAVAILABLE", "Не настроены защищённые сессии.", false, nil)
				return
			}
			actor, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
			if !ok {
				return
			}
			// Do not mutate a remote UCR room before validating local authority and
			// the canonical APGIC business lifecycle/time window.
			instant := now().UTC()
			id := r.PathValue("eventID")
			current, err := store.LiveEvent(r.Context(), id, actor)
			if err != nil {
				liveError(w, r, err)
				return
			}
			if !liveevents.CanTransition(current.State, transition.target) {
				liveError(w, r, liveevents.ErrConflict)
				return
			}
			if (transition.target == liveevents.Published && !current.StartsAt.After(instant)) ||
				(transition.target == liveevents.Live && (instant.Before(current.StartsAt.Add(-15*time.Minute)) || instant.After(current.EndsAt))) ||
				(transition.target == liveevents.Completed && instant.Before(current.StartsAt)) {
				liveError(w, r, liveevents.ErrConflict)
				return
			}
			// Draft cancellation is purely APGIC business state. Published/live
			// transitions require successful UCR media lifecycle acknowledgment.
			if !(transition.target == liveevents.Cancelled && current.State == liveevents.Draft) {
				service, ok := store.(LiveMediaStore)
				if !ok || media == nil {
					liveError(w, r, liveevents.ErrUnavailable)
					return
				}
				room, err := service.LiveMediaForOwner(r.Context(), actor, id)
				if err != nil {
					liveError(w, r, err)
					return
				}
				// Never advance a real UCR session if canonical SQL will reject
				// an expired readiness proof afterwards.
				if (transition.target == liveevents.Published || transition.target == liveevents.Live) &&
					(room.ProviderReadyAt.IsZero() || instant.Sub(room.ProviderReadyAt) > 5*time.Minute ||
					room.ProviderReadyAt.After(instant.Add(time.Minute))) {
					liveError(w, r, liveevents.ErrUnavailable)
					return
				}
				if err = media.Change(r.Context(), room, transition.target); err != nil {
					liveError(w, r, err)
					return
				}
			}
			event, err := store.ChangeLiveEvent(r.Context(), actor, id, transition.target, instant)
			if err != nil {
				liveError(w, r, err)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			writeJSON(w, 200, event)
		})
	}
	mux.HandleFunc("POST /v1/live-events/{eventID}/registrations", func(w http.ResponseWriter, r *http.Request) {
		if !liveMutationAllowed(r) {
			liveError(w, r, liveevents.ErrForbidden)
			return
		}
		if store == nil {
			liveError(w, r, liveevents.ErrUnavailable)
			return
		}
		if sessions == nil || sessionConfigErr != nil {
			writeDemandError(w, r, 503, "CLIENT_SESSION_UNAVAILABLE", "Не настроены защищённые сессии.", false, nil)
			return
		}
		actor, cookie, err := sessions.identityForCreate(r)
		if err != nil {
			writeDemandError(w, r, 401, "CLIENT_SESSION_INVALID", "Недействительная сессия.", false, nil)
			return
		}
		registered, err := store.RegisterLiveEvent(r.Context(), actor, r.PathValue("eventID"), now().UTC())
		if err != nil {
			liveError(w, r, err)
			return
		}
		if cookie != nil {
			http.SetCookie(w, cookie)
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 201, registered)
	})
	mux.HandleFunc("GET /v1/live-events/{eventID}/registration", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			liveError(w, r, liveevents.ErrUnavailable)
			return
		}
		if sessions == nil || sessionConfigErr != nil {
			writeDemandError(w, r, 503, "CLIENT_SESSION_UNAVAILABLE", "Не настроены защищённые сессии.", false, nil)
			return
		}
		actor, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		registered, err := store.MyLiveRegistration(r.Context(), actor, r.PathValue("eventID"))
		if err != nil {
			liveError(w, r, err)
			return
		}
		if registered == nil {
			liveError(w, r, liveevents.ErrNotFound)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, registered)
	})
	mux.HandleFunc("POST /v1/live-events/{eventID}/unregister", func(w http.ResponseWriter, r *http.Request) {
		if !liveMutationAllowed(r) {
			liveError(w, r, liveevents.ErrForbidden)
			return
		}
		if store == nil {
			liveError(w, r, liveevents.ErrUnavailable)
			return
		}
		if sessions == nil || sessionConfigErr != nil {
			writeDemandError(w, r, 503, "CLIENT_SESSION_UNAVAILABLE", "Не настроены защищённые сессии.", false, nil)
			return
		}
		actor, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		result, err := store.CancelLiveRegistration(r.Context(), actor, r.PathValue("eventID"), now().UTC())
		if err != nil {
			liveError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, result)
	})
	mux.HandleFunc("GET /v1/live-events/{eventID}/access", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			liveError(w, r, liveevents.ErrUnavailable)
			return
		}
		if sessions == nil || sessionConfigErr != nil {
			writeDemandError(w, r, 503, "CLIENT_SESSION_UNAVAILABLE", "Не настроены защищённые сессии.", false, nil)
			return
		}
		actor, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		access, err := store.LiveEventAccess(r.Context(), actor, r.PathValue("eventID"), now().UTC())
		if err != nil {
			liveError(w, r, err)
			return
		}
		// This is eligibility only, NOT a provider-issued media ticket.
		access.Allowed = false
		if access.ReasonCode == "LIVE_JOIN_ELIGIBLE" {
			access.ReasonCode = "LIVE_PROVIDER_TICKET_NOT_CONFIGURED"
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, access)
	})
	mux.HandleFunc("POST /v1/live-provider-webhooks", func(w http.ResponseWriter, r *http.Request) {
		if store == nil || keys == nil {
			liveError(w, r, liveevents.ErrUnavailable)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		var envelope connector.WebhookEnvelope
		if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
			writeDemandError(w, r, 401, "LIVE_PROVIDER_EVIDENCE_UNVERIFIED", "Подпись провайдера не подтверждена.", false, nil)
			return
		}
		if err := connector.VerifyWebhook(envelope, keys); err != nil {
			writeDemandError(w, r, 401, "LIVE_PROVIDER_EVIDENCE_UNVERIFIED", "Подпись провайдера не подтверждена.", false, nil)
			return
		}
		decision, err := store.ApplyLiveProviderWebhook(r.Context(), envelope, keys)
		if err != nil {
			liveError(w, r, err)
			return
		}
		writeJSON(w, 200, map[string]string{"delivery_decision": string(decision)})
	})
}
