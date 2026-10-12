package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/liveevents"
)

// SQL authorization is always re-checked on the server for each media action.
// The browser never sees machine credentials, internal conference grants, or
// raw connector credentials.
type LiveMediaStore interface {
	LiveMediaSetupEvent(context.Context, string, string) (liveevents.Event, error)
	BindLiveMedia(context.Context, string, string, liveevents.MediaRoom) (liveevents.MediaRoom, error)
	LiveMediaForOwner(context.Context, string, string) (liveevents.MediaRoom, error)
	LiveMediaForJoin(context.Context, string, string, time.Time) (liveevents.MediaRoom, string, error)
	LiveMediaRegistered(context.Context, string, string) ([]string, error)
	BanLiveParticipant(context.Context, string, string, string, time.Time) error
	SaveLiveAttendance(context.Context, liveevents.MediaRoom, string, liveevents.MediaParticipantFact, time.Time) error
}

func registerLiveMedia(mux *http.ServeMux, store LiveEventStore, media liveevents.RealMediaProvider,
	sessions *clientSessionManager, sessionConfigErr error, now func() time.Time) {
	authenticate := func(w http.ResponseWriter, r *http.Request) (string, bool) {
		if !liveMutationAllowed(r) {
			liveError(w, r, liveevents.ErrForbidden)
			return "", false
		}
		if sessions == nil || sessionConfigErr != nil {
			writeDemandError(w, r, 503, "CLIENT_SESSION_UNAVAILABLE", "Не настроены защищённые сессии.", false, nil)
			return "", false
		}
		return trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
	}
	check := func(w http.ResponseWriter, r *http.Request) (LiveMediaStore, bool) {
		if store == nil || media == nil {
			liveError(w, r, liveevents.ErrUnavailable)
			return nil, false
		}
		service, ok := store.(LiveMediaStore)
		if !ok {
			liveError(w, r, liveevents.ErrUnavailable)
			return nil, false
		}
		return service, true
	}
	mux.HandleFunc("GET /v1/live-events/{eventID}/participants", func(w http.ResponseWriter, r *http.Request) {
		// A private owner-only moderation roster. Never a public attendee directory.
		if sessions == nil || sessionConfigErr != nil {
			writeDemandError(w, r, 503, "CLIENT_SESSION_UNAVAILABLE", "Не настроены защищённые сессии.", false, nil)
			return
		}
		actor, ok := trustedClientIdentity(w, r, sessions, sessionConfigErr, "")
		if !ok {
			return
		}
		service, ok := check(w, r)
		if !ok {
			return
		}
		ids, err := service.LiveMediaRegistered(r.Context(), actor, r.PathValue("eventID"))
		if err != nil {
			liveError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, 200, map[string]any{"identities": ids})
	})
	mux.HandleFunc("POST /v1/live-events/{eventID}/prepare-media", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticate(w, r)
		if !ok {
			return
		}
		service, ok := check(w, r)
		if !ok {
			return
		}
		event, err := service.LiveMediaSetupEvent(r.Context(), actor, r.PathValue("eventID"))
		if err != nil {
			liveError(w, r, err)
			return
		}
		if event.State != liveevents.Draft && event.State != liveevents.Published && event.State != liveevents.Live {
			liveError(w, r, liveevents.ErrConflict)
			return
		}
		room, err := media.Prepare(r.Context(), event)
		if err != nil {
			liveError(w, r, err)
			return
		}
		saved, err := service.BindLiveMedia(r.Context(), actor, event.ID, room)
		if err != nil {
			liveError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, map[string]any{"event_id": saved.EventID, "provider_ready": true, "conference_bound": true})
	})
	mux.HandleFunc("POST /v1/live-events/{eventID}/join", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticate(w, r)
		if !ok {
			return
		}
		service, ok := check(w, r)
		if !ok {
			return
		}
		room, role, err := service.LiveMediaForJoin(r.Context(), actor, r.PathValue("eventID"), now().UTC())
		if err != nil {
			liveError(w, r, err)
			return
		}
		grant, err := media.Join(r.Context(), room, actor, role)
		if err != nil {
			liveError(w, r, err)
			return
		}
		if grant.JoinURL == "" || !grant.ExpiresAt.After(now()) {
			liveError(w, r, liveevents.ErrUnavailable)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Referrer-Policy", "no-referrer")
		writeJSON(w, 200, map[string]any{"join_url": grant.JoinURL, "expires_at": grant.ExpiresAt, "role": role})
	})
	mux.HandleFunc("POST /v1/live-events/{eventID}/moderate", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticate(w, r)
		if !ok {
			return
		}
		service, ok := check(w, r)
		if !ok {
			return
		}
		room, err := service.LiveMediaForOwner(r.Context(), actor, r.PathValue("eventID"))
		if err != nil {
			liveError(w, r, err)
			return
		}
		if room.State != liveevents.Live && room.State != liveevents.Published {
			liveError(w, r, liveevents.ErrConflict)
			return
		}
		var input struct {
			IdentityID string
			Action     string
		}
		var payload map[string]string
		if err := decodeLiveJSON(w, r, &payload); err != nil {
			liveError(w, r, liveevents.ErrInvalid)
			return
		}
		input.IdentityID = strings.TrimSpace(payload["identity_id"])
		input.Action = strings.TrimSpace(payload["action"])
		if input.IdentityID == "" || input.Action != "mute" && input.Action != "remove" {
			liveError(w, r, liveevents.ErrInvalid)
			return
		}
		participants, err := service.LiveMediaRegistered(r.Context(), actor, room.EventID)
		if err != nil {
			liveError(w, r, err)
			return
		}
		found := false
		for _, id := range participants {
			if id == input.IdentityID {
				found = true
				break
			}
		}
		if !found {
			liveError(w, r, liveevents.ErrNotFound)
			return
		}
		// A remove request first closes APGIC's admission policy. If UCR is
		// unreachable afterwards, no second join grant can be minted while
		// the provider action is retried and reconciled.
		if input.Action == "remove" {
			if err = service.BanLiveParticipant(r.Context(), actor, room.EventID, input.IdentityID, now().UTC()); err != nil {
				liveError(w, r, err)
				return
			}
		}
		if err = media.Moderate(r.Context(), room, input.IdentityID, input.Action); err != nil {
			liveError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, 200, map[string]any{"confirmed": true, "action": input.Action})
	})
	mux.HandleFunc("POST /v1/live-events/{eventID}/attendance/sync", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticate(w, r)
		if !ok {
			return
		}
		service, ok := check(w, r)
		if !ok {
			return
		}
		room, err := service.LiveMediaForOwner(r.Context(), actor, r.PathValue("eventID"))
		if err != nil {
			liveError(w, r, err)
			return
		}
		if room.State != liveevents.Published && room.State != liveevents.Live && room.State != liveevents.Completed {
			liveError(w, r, liveevents.ErrConflict)
			return
		}
		participants, err := service.LiveMediaRegistered(r.Context(), actor, room.EventID)
		if err != nil {
			liveError(w, r, err)
			return
		}
		joined := 0
		for _, id := range participants {
			fact, err := media.Attendance(r.Context(), room, id)
			if err != nil {
				liveError(w, r, err)
				return
			}
			if fact.Joined {
				if err = service.SaveLiveAttendance(r.Context(), room, id, fact, now().UTC()); err != nil {
					liveError(w, r, err)
					return
				}
				joined++
			}
		}
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, 200, map[string]any{"registered": len(participants), "provider_verified_attendees": joined})
	})
}
