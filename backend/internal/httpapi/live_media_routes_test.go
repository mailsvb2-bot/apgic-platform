package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/liveevents"
)

type r9MediaDBStub struct {
	LiveEventStore
	event  liveevents.Event
	room   liveevents.MediaRoom
	actor  string
	bound  bool
	banned bool
	saved  bool
}

func (s *r9MediaDBStub) LiveMediaSetupEvent(_ context.Context, actor, eventID string) (liveevents.Event, error) {
	if actor != s.actor || eventID != s.event.ID {
		return liveevents.Event{}, liveevents.ErrForbidden
	}
	return s.event, nil
}
func (s *r9MediaDBStub) LiveEvent(_ context.Context, eventID, actor string) (liveevents.Event, error) {
	if eventID != s.event.ID {
		return liveevents.Event{}, liveevents.ErrNotFound
	}
	return s.event, nil
}
func (s *r9MediaDBStub) BindLiveMedia(_ context.Context, actor, eventID string, room liveevents.MediaRoom) (liveevents.MediaRoom, error) {
	if actor != s.actor || eventID != s.event.ID {
		return liveevents.MediaRoom{}, liveevents.ErrForbidden
	}
	s.room = room
	s.bound = true
	return room, nil
}
func (s *r9MediaDBStub) LiveMediaForOwner(_ context.Context, actor, eventID string) (liveevents.MediaRoom, error) {
	if actor != s.actor || eventID != s.event.ID {
		return liveevents.MediaRoom{}, liveevents.ErrForbidden
	}
	if !s.bound {
		return liveevents.MediaRoom{}, liveevents.ErrUnavailable
	}
	return s.room, nil
}
func (s *r9MediaDBStub) LiveMediaForJoin(_ context.Context, actor, eventID string, now time.Time) (liveevents.MediaRoom, string, error) {
	if eventID != s.event.ID || !s.bound || s.banned {
		return liveevents.MediaRoom{}, "", liveevents.ErrForbidden
	}
	if actor == s.actor {
		return s.room, "host", nil
	}
	if actor != "11111111-1111-4111-8111-111111111111" {
		return liveevents.MediaRoom{}, "", liveevents.ErrForbidden
	}
	return s.room, "attendee", nil
}
func (s *r9MediaDBStub) LiveMediaRegistered(_ context.Context, actor, eventID string) ([]string, error) {
	if actor != s.actor || eventID != s.event.ID {
		return nil, liveevents.ErrForbidden
	}
	return []string{"11111111-1111-4111-8111-111111111111"}, nil
}
func (s *r9MediaDBStub) BanLiveParticipant(_ context.Context, actor, eventID, identity string, now time.Time) error {
	if actor != s.actor || eventID != s.event.ID || identity != "11111111-1111-4111-8111-111111111111" {
		return liveevents.ErrForbidden
	}
	s.banned = true
	return nil
}
func (s *r9MediaDBStub) SaveLiveAttendance(_ context.Context, room liveevents.MediaRoom, identity string, fact liveevents.MediaParticipantFact, at time.Time) error {
	if !fact.Joined {
		return liveevents.ErrInvalid
	}
	s.saved = true
	return nil
}
func (s *r9MediaDBStub) ChangeLiveEvent(_ context.Context, actor, eventID string, target liveevents.State, now time.Time) (liveevents.Event, error) {
	if actor != s.actor {
		return liveevents.Event{}, liveevents.ErrForbidden
	}
	s.event.State = target
	s.room.State = target
	return s.event, nil
}

type r9MediaProviderStub struct {
	prepared        int
	joins           int
	transitions     int
	moderated       int
	attendanceCalls int
}

func (m *r9MediaProviderStub) Prepare(_ context.Context, event liveevents.Event) (liveevents.MediaRoom, error) {
	m.prepared++
	return liveevents.MediaRoom{EventID: event.ID, OrganizationID: event.OrganizationID, ProviderInstanceID: "connector",
		ConferenceID: "UCR-real-room-from-provider", IntegrationID: "ucr-integration",
		ProviderReadyAt: time.Now().UTC(), StartsAt: event.StartsAt, EndsAt: event.EndsAt, Capacity: event.Capacity,
		Format: event.Format, State: event.State}, nil
}
func (m *r9MediaProviderStub) Change(_ context.Context, room liveevents.MediaRoom, to liveevents.State) error {
	m.transitions++
	return nil
}
func (m *r9MediaProviderStub) Join(_ context.Context, room liveevents.MediaRoom, identity, role string) (liveevents.MediaGrant, error) {
	m.joins++
	return liveevents.MediaGrant{JoinURL: "https://join.example/room#ucr_join=provider-token", ExpiresAt: time.Now().Add(time.Minute)}, nil
}
func (m *r9MediaProviderStub) Moderate(_ context.Context, room liveevents.MediaRoom, identity, action string) error {
	m.moderated++
	return nil
}
func (m *r9MediaProviderStub) Attendance(_ context.Context, room liveevents.MediaRoom, identity string) (liveevents.MediaParticipantFact, error) {
	m.attendanceCalls++
	return liveevents.MediaParticipantFact{
		Joined: true, MediaReady: true, JoinCount: 1, MediaReadyCount: 1, ConnectedSeconds: 55,
		FirstJoinedAt: time.Now().Add(-time.Minute), FirstMediaReadyAt: time.Now().Add(-time.Second),
	}, nil
}
func TestR9MediaJourneyBoundaryAndNoUnauthorizedJoin(t *testing.T) {
	owner := "22222222-2222-4222-8222-222222222222"
	participant := "11111111-1111-4111-8111-111111111111"
	eventID := "33333333-3333-4333-8333-333333333333"
	db := &r9MediaDBStub{actor: owner, event: liveevents.Event{
		ID: eventID, OrganizationID: "44444444-4444-4444-8444-444444444444", State: liveevents.Draft,
		StartsAt: time.Now().Add(time.Hour), EndsAt: time.Now().Add(2 * time.Hour), Capacity: 6, Format: liveevents.Group}}
	media := &r9MediaProviderStub{}
	h := New(Options{LiveEventsEnabled: true, LiveEvents: db, LiveMedia: media,
		ClientSessionKey: []byte(strings.Repeat("s", 40))})
	post := func(actor, path, body string) (int, string) {
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if actor != "" {
			req.AddCookie(issueLiveCookie(t, actor))
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w.Code, w.Body.String()
	}
	path := "/v1/live-events/" + eventID
	if code, _ := post("", path+"/prepare-media", ""); code != 401 || media.prepared != 0 {
		t.Fatal("anonymous created media")
	}
	if code, _ := post(participant, path+"/prepare-media", ""); code != 403 || media.prepared != 0 {
		t.Fatal("foreign user created media")
	}
	if code, b := post(owner, path+"/prepare-media", ""); code != 200 || !db.bound {
		t.Fatalf("prepare: %d %s", code, b)
	}
	if code, b := post(owner, path+"/publish", ""); code != 200 || media.transitions != 1 {
		t.Fatalf("publish: %d %s", code, b)
	}
	if code, b := post("", path+"/join", ""); code != 401 || media.joins != 0 {
		t.Fatalf("anonymous joined: %d %s", code, b)
	}
	if code, b := post("55555555-5555-4555-8555-555555555555", path+"/join", ""); code != 403 || media.joins != 0 {
		t.Fatalf("foreigner joined: %d %s", code, b)
	}
	if code, b := post(participant, path+"/join", ""); code != 200 || !strings.Contains(b, "ucr_join=") || media.joins != 1 {
		t.Fatalf("joined: %d %s", code, b)
	}
	if code, b := post(owner, path+"/attendance/sync", ""); code != 200 || !db.saved || media.attendanceCalls != 1 {
		t.Fatalf("attendance: %d %s", code, b)
	}
	if code, b := post(participant, path+"/moderate", `{"identity_id":"11111111-1111-4111-8111-111111111111","action":"remove"}`); code != 403 || media.moderated != 0 {
		t.Fatalf("user moderated: %d %s", code, b)
	}
	if code, b := post(owner, path+"/moderate", `{"identity_id":"11111111-1111-4111-8111-111111111111","action":"remove"}`); code != 200 || !db.banned || media.moderated != 1 {
		t.Fatalf("owner moderation: %d %s", code, b)
	}
	if code, _ := post(participant, path+"/join", ""); code != 403 || media.joins != 1 {
		t.Fatal("removed user rejoined")
	}
}
func TestR9MediaRoutesNeverClaimJoinWithoutProvider(t *testing.T) {
	owner := "22222222-2222-4222-8222-222222222222"
	eventID := "33333333-3333-4333-8333-333333333333"
	db := &r9MediaDBStub{actor: owner}
	h := New(Options{LiveEventsEnabled: true, LiveEvents: db, ClientSessionKey: []byte(strings.Repeat("s", 40))})
	req := httptest.NewRequest("POST", "/v1/live-events/"+eventID+"/join", nil)
	req.AddCookie(issueLiveCookie(t, owner))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable || strings.Contains(rec.Body.String(), "join_url") {
		t.Fatalf("provider missing: %d %s", rec.Code, rec.Body.String())
	}
}
