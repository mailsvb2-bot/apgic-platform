package httpapi

import (
	"context"
	"crypto/ed25519"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/connector"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/liveevents"
)

type liveEventStub struct {
	LiveEventStore // non-invoked methods deliberately panic on unexpected route
	actor          string
	registerCount  int
}

func (s *liveEventStub) RegisterLiveEvent(_ context.Context, actor, id string, at time.Time) (liveevents.Registration, error) {
	s.actor = actor
	s.registerCount++
	return liveevents.Registration{ID: "reg-test", EventID: id, IdentityID: actor, State: "REGISTERED", RegisteredAt: at}, nil
}
func (s *liveEventStub) LiveEventAccess(_ context.Context, actor, id string, at time.Time) (liveevents.Access, error) {
	s.actor = actor
	return liveevents.Access{Allowed: true, ProviderReady: true, ReasonCode: "LIVE_JOIN_ELIGIBLE"}, nil
}
func (s *liveEventStub) CreateLiveEvent(_ context.Context, actor, org, idem string, in liveevents.DraftInput, now time.Time) (liveevents.Event, error) {
	s.actor = actor
	return liveevents.Event{ID: "created", OrganizationID: org, Title: in.Title, State: liveevents.Draft}, nil
}
func issueLiveCookie(t *testing.T, id string) *http.Cookie {
	t.Helper()
	session, err := newClientSessionManager([]byte(strings.Repeat("s", 40)), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := session.issue(id)
	if err != nil {
		t.Fatal(err)
	}
	return cookie
}
func TestLiveEventsOffByDefault(t *testing.T) {
	for _, path := range []string{"/v1/live-events", "/v1/live-events/a/registrations", "/v1/live-provider-webhooks"} {
		request := httptest.NewRequest("GET", path, nil)
		response := httptest.NewRecorder()
		New(Options{}).ServeHTTP(response, request)
		if response.Code != 404 {
			t.Errorf("%s: want 404 got %d", path, response.Code)
		}
	}
}
func TestLiveEventsFailClosedWithoutStorage(t *testing.T) {
	h := New(Options{LiveEventsEnabled: true})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/v1/live-events", nil))
	if w.Code != 503 || !strings.Contains(w.Body.String(), "LIVE_PROVIDER_UNAVAILABLE") {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
}
func TestLiveRegistrationRequiresSessionAndDoesNotTrustBody(t *testing.T) {
	stub := &liveEventStub{}
	h := New(Options{LiveEventsEnabled: true, LiveEvents: stub, ClientSessionKey: []byte(strings.Repeat("s", 40))})
	absent := httptest.NewRecorder()
	h.ServeHTTP(absent, httptest.NewRequest("GET", "/v1/live-events/a/access", nil))
	if absent.Code != 401 || stub.actor != "" {
		t.Fatalf("unauthorized access %d", absent.Code)
	}
	actor := "11111111-1111-4111-8111-111111111111"
	req := httptest.NewRequest("POST", "/v1/live-events/a/registrations", nil)
	req.AddCookie(issueLiveCookie(t, actor))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 201 || stub.actor != actor || stub.registerCount != 1 {
		t.Fatalf("registrations %d actor=%q body=%s", w.Code, stub.actor, w.Body.String())
	}
}
func TestLiveAccessNeverFabricatesProviderTicket(t *testing.T) {
	stub := &liveEventStub{}
	h := New(Options{LiveEventsEnabled: true, LiveEvents: stub, ClientSessionKey: []byte(strings.Repeat("s", 40))})
	req := httptest.NewRequest("GET", "/v1/live-events/event/access", nil)
	req.AddCookie(issueLiveCookie(t, "11111111-1111-4111-8111-111111111111"))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 || strings.Contains(w.Body.String(), `"allowed":true`) ||
		!strings.Contains(w.Body.String(), "LIVE_PROVIDER_TICKET_NOT_CONFIGURED") {
		t.Fatalf("failed closed media ticket: %d %s", w.Code, w.Body.String())
	}
}
func TestLiveUnsafeOriginAndPaidAttemptsRejected(t *testing.T) {
	stub := &liveEventStub{}
	h := New(Options{LiveEventsEnabled: true, LiveEvents: stub, ClientSessionKey: []byte(strings.Repeat("s", 40))})
	actor := "11111111-1111-4111-8111-111111111111"
	for _, tc := range []struct{ name, origin, payload, reason string }{
		{"cross site", "https://attacker.example", `{"title":"Допустимый вебинар"}`, "LIVE_EVENT_FORBIDDEN"},
		{"untrusted fields", "", "not-json", "LIVE_EVENT_INVALID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/v1/organizations/org/live-events", strings.NewReader(tc.payload))
			req.AddCookie(issueLiveCookie(t, actor))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", "test-idempotency-key-long")
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if !strings.Contains(w.Body.String(), tc.reason) {
				t.Fatalf("status %d %s", w.Code, w.Body.String())
			}
		})
	}
}

type rejectLiveResolver struct{}

func (rejectLiveResolver) ResolveWebhookPublicKey(string, string) (ed25519.PublicKey, bool) {
	return nil, false
}
func TestLiveUnsignedProviderFactsCannotChangeAttendance(t *testing.T) {
	stub := &liveEventStub{}
	h := New(Options{LiveEventsEnabled: true, LiveEvents: stub, ProviderWebhookKeys: rejectLiveResolver{}})
	req := httptest.NewRequest("POST", "/v1/live-provider-webhooks", strings.NewReader(`{"connector_instance_id":"fake","payload":{}}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 401 || !strings.Contains(w.Body.String(), "LIVE_PROVIDER_EVIDENCE_UNVERIFIED") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

var _ connector.WebhookPublicKeyResolver = rejectLiveResolver{}

func (s *liveEventStub) ListOrganizationLiveEvents(_ context.Context, actor, org string) ([]liveevents.Event, error) {
	s.actor = actor
	if org != "00000000-0000-4000-8000-000000000010" {
		return nil, liveevents.ErrForbidden
	}
	return []liveevents.Event{{ID: "owned", OrganizationID: org, Title: "Only owner draft", State: liveevents.Draft}}, nil
}

func TestLiveOrganizationListNeverTrustsCallerSuppliedIdentity(t *testing.T) {
	stub := &liveEventStub{}
	handler := New(Options{LiveEventsEnabled: true, LiveEvents: stub, ClientSessionKey: []byte(strings.Repeat("s", 40))})
	path := "/v1/organizations/00000000-0000-4000-8000-000000000010/live-events"
	anonymous := httptest.NewRecorder()
	handler.ServeHTTP(anonymous, httptest.NewRequest("GET", path, nil))
	if anonymous.Code != 401 || stub.actor != "" {
		t.Fatalf("anonymous owner list %d actor=%q", anonymous.Code, stub.actor)
	}
	identity := "11111111-1111-4111-8111-111111111111"
	request := httptest.NewRequest("GET", path, nil)
	request.AddCookie(issueLiveCookie(t, identity))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 200 || stub.actor != identity || !strings.Contains(response.Body.String(), "Only owner draft") {
		t.Fatalf("owner list %d actor=%q body=%s", response.Code, stub.actor, response.Body.String())
	}
	foreign := httptest.NewRequest("GET", "/v1/organizations/00000000-0000-4000-8000-000000000020/live-events", nil)
	foreign.AddCookie(issueLiveCookie(t, identity))
	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, foreign)
	if denied.Code != 403 {
		t.Fatalf("foreign tenant list %d body=%s", denied.Code, denied.Body.String())
	}
}

func (s *liveEventStub) ReadLiveEventAttendance(_ context.Context, actor, org, event string) (liveevents.AttendanceSummary, error) {
	s.actor = actor
	if org != "00000000-0000-4000-8000-000000000010" {
		return liveevents.AttendanceSummary{}, liveevents.ErrForbidden
	}
	return liveevents.AttendanceSummary{EventID: event, Registered: 2, JoinedParticipants: 1, ProviderJoinFacts: 1, VerifiedEvidence: true}, nil
}

func TestLiveAttendanceAggregateOwnerBoundary(t *testing.T) {
	stub := &liveEventStub{}
	handler := New(Options{LiveEventsEnabled: true, LiveEvents: stub, ClientSessionKey: []byte(strings.Repeat("s", 40))})
	identity := "11111111-1111-4111-8111-111111111111"
	path := "/v1/organizations/00000000-0000-4000-8000-000000000010/live-events/test/attendance"
	req := httptest.NewRequest("GET", path, nil)
	req.AddCookie(issueLiveCookie(t, identity))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "\"joined_participants\":1") || stub.actor != identity {
		t.Fatalf("attendance scope status=%d actor=%s body=%s", rec.Code, stub.actor, rec.Body.String())
	}
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest("GET", path, nil))
	if unauthorized.Code != 401 {
		t.Fatalf("anonymous attendance status=%d body=%s", unauthorized.Code, unauthorized.Body.String())
	}
	foreign := httptest.NewRequest("GET", "/v1/organizations/00000000-0000-4000-8000-000000000020/live-events/test/attendance", nil)
	foreign.AddCookie(issueLiveCookie(t, identity))
	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, foreign)
	if denied.Code != 403 {
		t.Fatalf("foreign attendance status=%d body=%s", denied.Code, denied.Body.String())
	}
}
