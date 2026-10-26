package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/instant"
)

type fakeDutyStore struct {
	owner    string
	dispatch instant.Dispatch
}

func (s *fakeDutyStore) SetPresence(id, offer string, available bool) (instant.Presence, error) {
	s.owner = id
	return instant.Presence{SpecialistIdentityID: id, OfferID: offer, State: instant.StateAvailable}, nil
}
func (s *fakeDutyStore) GetPresence(id string) (instant.Presence, error) {
	s.owner = id
	return instant.Presence{SpecialistIdentityID: id, State: instant.StateOffline}, nil
}
func (s *fakeDutyStore) Request(id, intent, key string) (instant.Dispatch, error) {
	s.owner = id
	s.dispatch = instant.Dispatch{ID: "3f898240-2092-4922-a582-e001cd9e378b", RequestKey: key,
		ClientIdentityID: id, HelpIntentID: intent, State: instant.DispatchOffered}
	s.dispatch.Normalize()
	return s.dispatch, nil
}
func (s *fakeDutyStore) GetDispatch(id, owner string) (instant.Dispatch, error) {
	if s.dispatch.ID != id || s.dispatch.ClientIdentityID != owner {
		return instant.Dispatch{}, instant.ErrNotFound
	}
	return s.dispatch, nil
}
func (s *fakeDutyStore) ActiveDispatch(identityID string) (instant.Dispatch, error) {
	if identityID != s.dispatch.ClientIdentityID || s.dispatch.ID == "" {
		return instant.Dispatch{}, instant.ErrNotFound
	}
	return s.dispatch, nil
}

func (s *fakeDutyStore) Cancel(id, clientIdentityID string) (instant.Dispatch, error) {
	if s.dispatch.ID != id || s.dispatch.ClientIdentityID != clientIdentityID {
		return instant.Dispatch{}, instant.ErrNotFound
	}
	if s.dispatch.State != instant.DispatchOffered && s.dispatch.State != instant.DispatchAccepted && s.dispatch.State != instant.DispatchCancelled {
		return instant.Dispatch{}, instant.ErrConflict
	}
	s.dispatch.State = instant.DispatchCancelled
	s.dispatch.Normalize()
	return s.dispatch, nil
}

func (s *fakeDutyStore) Answer(id, owner string, accept bool) (instant.Dispatch, error) {
	if id != s.dispatch.ID {
		return instant.Dispatch{}, instant.ErrNotFound
	}
	s.dispatch.State = instant.DispatchAccepted
	s.dispatch.Normalize()
	return s.dispatch, nil
}
func (s *fakeDutyStore) Inbox(id string) ([]instant.Dispatch, error) {
	s.owner = id
	return []instant.Dispatch{}, nil
}

func TestDutyDisabledAndFailClosedWithoutSession(t *testing.T) {
	store := &fakeDutyStore{}
	handler := New(Options{Instant: store, InstantEnabled: false})
	req := httptest.NewRequest(http.MethodGet, "/v1/instant/presence", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("disabled duty: %d", rr.Code)
	}
	handler = New(Options{Instant: store, InstantEnabled: true})
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("duty without trusted sessions: %d", rr.Code)
	}
}

func TestDutyTrustsCookieNotCallerProvidedIdentity(t *testing.T) {
	store := &fakeDutyStore{}
	key := []byte(strings.Repeat("s", 32))
	identity := "61f998fe-cd75-4e88-bdc7-2bfc7dc2a07a"
	session, err := newClientSessionManager(key, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := session.issue(identity)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(Options{ClientSessionKey: key, Instant: store, InstantEnabled: true})
	req := httptest.NewRequest(http.MethodPut, "/v1/instant/presence",
		strings.NewReader(`{"available":true,"offer_id":"f5e7f8e5-e7f5-4b7e-a480-4a407b14daa1","identity_id":"ATTACKER"}`))
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest || store.owner != "" {
		t.Fatalf("forged identity was accepted: status=%d owner=%s", rr.Code, store.owner)
	}
	// The canonical identity must still come from the signed cookie, never a
	// caller-controlled body field.
	req = httptest.NewRequest(http.MethodPut, "/v1/instant/presence",
		strings.NewReader(`{"available":true,"offer_id":"f5e7f8e5-e7f5-4b7e-a480-4a407b14daa1"}`))
	req.AddCookie(cookie)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || store.owner != identity {
		t.Fatalf("signed cookie owner not enforced: status=%d owner=%s", rr.Code, store.owner)
	}
	req = httptest.NewRequest(http.MethodPost, "/v1/instant/dispatches",
		strings.NewReader(`{"help_intent_id":"140d5ba9-d001-47d2-92df-75802170a9fb","request_key":"4030e5e3-12f0-479c-9cb2-dffcc8c1adf2"}`))
	req.AddCookie(cookie)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("request failed: %d %s", rr.Code, rr.Body.String())
	}
	var d instant.Dispatch
	if err := json.Unmarshal(rr.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if d.RoomReady || d.ClientIdentityID != identity || d.NextAction != instant.ActionWaiting {
		t.Fatalf("unsafe dispatch %+v", d)
	}
}

func TestDutyCannotAnswerWithoutExplicitBooleanOrOwnership(t *testing.T) {
	store := &fakeDutyStore{}
	key := []byte(strings.Repeat("s", 32))
	handler := New(Options{Instant: store, InstantEnabled: true, ClientSessionKey: key})
	req := httptest.NewRequest(http.MethodPost, "/v1/instant/dispatches/3f898240-2092-4922-a582-e001cd9e378b/answer",
		strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("missing session: %d", rr.Code)
	}
	session, err := newClientSessionManager(key, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := session.issue("61f998fe-cd75-4e88-bdc7-2bfc7dc2a07a")
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(cookie)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("implicit decline permitted: %d", rr.Code)
	}
}

func TestDutySpecialistProjectionNeverLeaksPrepaymentClientIdentity(t *testing.T) {
	raw := instant.Dispatch{
		ID:               "e0f74045-a5b4-4c63-b5d2-c222563a598a",
		ClientIdentityID: "private-client-id",
		HelpIntentID:     "private-help-intent-id",
		RequestKey:       "private-idempotency-key",
		TopicID:          "anxiety",
		PriceMinor:       12345,
		Currency:         "RUB",
		State:            instant.DispatchOffered,
		NextAction:       instant.ActionWaiting,
	}
	public, err := json.Marshal(dutySpecialistView(raw))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-client-id", "private-help-intent-id", "private-idempotency-key", "client_identity_id", "help_intent_id", "request_key"} {
		if strings.Contains(string(public), secret) {
			t.Fatalf("specialist response leaked %s: %s", secret, string(public))
		}
	}
	if !strings.Contains(string(public), "anxiety") {
		t.Fatalf("specialist lost safe matching facts: %s", string(public))
	}
}

func TestDutyCancellationOwnerAndNoUnpaidRoom(t *testing.T) {
	store := &fakeDutyStore{dispatch: instant.Dispatch{
		ID:               "3f898240-2092-4922-a582-e001cd9e378b",
		ClientIdentityID: "61f998fe-cd75-4e88-bdc7-2bfc7dc2a07a",
		State:            instant.DispatchAccepted,
	}}
	key := []byte(strings.Repeat("s", 32))
	session, err := newClientSessionManager(key, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	ownerCookie, err := session.issue(store.dispatch.ClientIdentityID)
	if err != nil {
		t.Fatal(err)
	}
	strangerCookie, err := session.issue("68c7c2a5-bf5b-4895-ac4a-d4d9944799ae")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(Options{ClientSessionKey: key, Instant: store, InstantEnabled: true})
	target := "/v1/instant/dispatches/" + store.dispatch.ID + "/cancel"
	req := httptest.NewRequest(http.MethodPost, target, nil)
	req.AddCookie(strangerCookie)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound || store.dispatch.State != instant.DispatchAccepted {
		t.Fatalf("non-owner cancellation changed state: %d %+v", w.Code, store.dispatch)
	}
	for attempt := 0; attempt < 2; attempt++ {
		req = httptest.NewRequest(http.MethodPost, target, nil)
		req.AddCookie(ownerCookie)
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("owner cancel attempt %d: %d %s", attempt, w.Code, w.Body.String())
		}
		var got instant.Dispatch
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.State != instant.DispatchCancelled || got.RoomReady || got.NextAction != instant.ActionRetry {
			t.Fatalf("cancelled duty granted unwanted access: %+v", got)
		}
	}
}

func (s *fakeDutyStore) ReadFulfillment(id, identity string) (instant.Fulfillment, error) {
	if id != s.dispatch.ID || identity != s.dispatch.ClientIdentityID {
		return instant.Fulfillment{}, instant.ErrNotFound
	}
	return instant.Fulfillment{
		DispatchID:      id,
		DispatchState:   s.dispatch.State,
		NextAction:      instant.ActionAwaitBooking,
		PaymentVerified: false,
		RoomReady:       false,
	}, nil
}

func TestDutyFulfillmentIsSessionOwnedAndDoesNotMintRoom(t *testing.T) {
	const owner = "61f998fe-cd75-4e88-bdc7-2bfc7dc2a07a"
	const stranger = "68c7c2a5-bf5b-4895-ac4a-d4d9944799ae"
	const id = "3f898240-2092-4922-a582-e001cd9e378b"
	store := &fakeDutyStore{dispatch: instant.Dispatch{
		ID: id, ClientIdentityID: owner, State: instant.DispatchAccepted,
	}}
	key := []byte(strings.Repeat("s", 32))
	sessions, err := newClientSessionManager(key, time.Now)
	if err != nil {
			t.Fatal(err)
		}
	handler := New(Options{Instant: store, InstantEnabled: true, ClientSessionKey: key})
	path := "/v1/instant/dispatches/" + id + "/fulfillment"
	read := func(identity string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if identity != "" {
			cookie, err := sessions.issue(identity)
			if err != nil {
			t.Fatal(err)
		}
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}
	if w := read(""); w.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned fulfillment read succeeded: %d", w.Code)
	}
	if w := read(stranger); w.Code != http.StatusNotFound {
		t.Fatalf("foreign fulfillment read succeeded: %d %s", w.Code, w.Body.String())
	}
	w := read(owner)
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("owner read failed: %d %s", w.Code, w.Body.String())
	}
	var status instant.Fulfillment
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.BookingID != "" || status.PaymentVerified || status.RoomReady {
		t.Fatalf("accepted R6 without canonical R2 minted access: %+v", status)
	}
}
