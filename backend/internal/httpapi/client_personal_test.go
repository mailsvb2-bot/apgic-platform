package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/clientpersonal"
)

type personalStoreFixture struct {
	readOwner   string
	writeOwner  string
	readCalls   int
	writeCalls  int
	preferences clientpersonal.Preferences
}

func (s *personalStoreFixture) ReadClientPersonal(_ context.Context, identity string, _ int, _ *clientpersonal.HistoryCursor, at time.Time) (clientpersonal.View, error) {
	s.readOwner = identity
	s.readCalls++
	return clientpersonal.View{
		IdentityID:  identity,
		AsOf:        at,
		Preferences: clientpersonal.DefaultPreferences(),
		Upcoming:    []clientpersonal.Booking{},
		History:     []clientpersonal.Booking{},
		Requests:    []clientpersonal.Request{},
	}, nil
}
func (s *personalStoreFixture) UpdateClientPersonalPreferences(_ context.Context, identity string, p clientpersonal.Preferences) (clientpersonal.Preferences, error) {
	s.writeOwner = identity
	s.writeCalls++
	s.preferences = p
	return p, nil
}

func TestPersonalAPISignedIdentityAndFailClosedAccess(t *testing.T) {
	const owner = "11111111-1111-4111-8111-111111111111"
	const foreign = "22222222-2222-4222-8222-222222222222"
	key := []byte(strings.Repeat("k", 48))
	st := &personalStoreFixture{}
	handler := New(Options{ClientPersonal: st, ClientSessionKey: key})
	sessions, err := newClientSessionManager(key, nil)
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := sessions.issue(owner)
	if err != nil {
		t.Fatal(err)
	}
	request := func(path string, withCookie bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		if withCookie {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}

	for _, path := range []string{"/v1/client/personal", "/v1/client/personal?history_cursor=forged", "/v1/client/personal?limit=999"} {
		out := request(path, false)
		if out.Code != http.StatusUnauthorized {
			t.Fatalf("no-session %s: %d: %s", path, out.Code, out.Body.String())
		}
	}
	if st.readCalls != 0 {
		t.Fatal("untrusted reads reached the store")
	}
	invalid := request("/v1/client/personal?identity_id="+foreign, true)
	if invalid.Code != http.StatusBadRequest || st.readCalls != 0 {
		t.Fatalf("caller identity accepted: %d", invalid.Code)
	}
	if request("/v1/client/personal?history_cursor=forged", true).Code != http.StatusBadRequest {
		t.Fatal("forged history cursor was accepted")
	}
	if request("/v1/client/personal?limit=999", true).Code != http.StatusBadRequest {
		t.Fatal("unbounded history was accepted")
	}
	valid := request("/v1/client/personal", true)
	if valid.Code != http.StatusOK || st.readOwner != owner || st.readCalls != 1 {
		t.Fatalf("signed view not owner scoped: status=%d owner=%s calls=%d", valid.Code, st.readOwner, st.readCalls)
	}
	if !strings.Contains(valid.Header().Get("Cache-Control"), "no-store") || strings.Contains(valid.Body.String(), foreign) {
		t.Fatal("private response may leak or be cached")
	}
}

func TestPersonalPreferencesStrictWriteWithoutIdentityOverride(t *testing.T) {
	const owner = "33333333-3333-4333-8333-333333333333"
	key := []byte(strings.Repeat("s", 48))
	st := &personalStoreFixture{}
	handler := New(Options{ClientPersonal: st, ClientSessionKey: key})
	sessions, err := newClientSessionManager(key, nil)
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := sessions.issue(owner)
	if err != nil {
		t.Fatal(err)
	}
	put := func(body string, signed bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPut, "/v1/client/personal/preferences", strings.NewReader(body))
		if signed {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	valid := `{"display_name":"Анна","locale":"ru","time_zone":"Europe/Moscow"}`
	if put(valid, false).Code != http.StatusUnauthorized || st.writeCalls != 0 {
		t.Fatal("anonymous mutation reached store")
	}
	for _, body := range []string{
		`{"display_name":"А","locale":"ru","time_zone":"Broken/Zone"}`,
		`{"display_name":"А","locale":"ru","time_zone":"UTC","identity_id":"other"}`,
		valid + "{}",
	} {
		response := put(body, true)
		if response.Code != http.StatusBadRequest || st.writeCalls != 0 {
			t.Fatalf("invalid preferences reached store: status=%d payload=%s", response.Code, body)
		}
	}
	response := put(valid, true)
	if response.Code != http.StatusOK || st.writeCalls != 1 || st.writeOwner != owner || st.preferences.DisplayName != "Анна" {
		t.Fatalf("personal preference owner mismatch: code=%d owner=%s", response.Code, st.writeOwner)
	}
	if !strings.Contains(response.Header().Get("Cache-Control"), "no-store") {
		t.Fatal("preferences may be cached")
	}
}

func (s *personalStoreFixture) CreateClientCheckIn(_ context.Context, identity string, v clientpersonal.CheckInInput) (clientpersonal.CheckIn, error) {
	s.writeOwner = identity
	s.writeCalls++
	return clientpersonal.CheckIn{CheckInInput: v, RecordedAt: time.Now().UTC()}, nil
}

func TestCheckInRejectsUnsignedAndClinicalClaims(t *testing.T) {
	key := []byte(strings.Repeat("q", 48))
	identity := "55555555-5555-4555-8555-555555555555"
	manager, _ := newClientSessionManager(key, nil)
	cookie, _ := manager.issue(identity)
	store := &personalStoreFixture{}
	handler := New(Options{ClientSessionKey: key, ClientPersonal: store})
	send := func(body string, signed bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/v1/client/personal/check-ins", strings.NewReader(body))
		if signed {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	valid := `{"booking_id":"123e4567-e89b-42d3-a456-426614174001","progress_code":"UNSURE"}`
	if v := send(valid, false); v.Code != 401 || store.writeCalls != 0 {
		t.Fatal("unsigned check-in accepted")
	}
	if v := send(`{"booking_id":"123e4567-e89b-42d3-a456-426614174001","progress_code":"CURED"}`, true); v.Code != 400 || store.writeCalls != 0 {
		t.Fatal("invalid assertion accepted")
	}
	if v := send(valid, true); v.Code != 201 || store.writeOwner != identity {
		t.Fatalf("owner path rejected: %d %s", v.Code, v.Body.String())
	}
}

func TestPersonalAPIFailsClosedWhenPersistenceUnavailable(t *testing.T) {
	key := []byte(strings.Repeat("p", 48))
	manager, _ := newClientSessionManager(key, nil)
	cookie, _ := manager.issue("44444444-4444-4444-8444-444444444444")
	handler := New(Options{ClientSessionKey: key})
	r := httptest.NewRequest(http.MethodGet, "/v1/client/personal", nil)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "PERSONAL_STORE_UNAVAILABLE") {
		t.Fatalf("no store must not mint history: %d %s", w.Code, w.Body.String())
	}
}

func TestPersistentAccountDeletionRejectsSyntheticErasureBeforeStoreMutation(t *testing.T) {
	const owner = "11111111-1111-4111-8111-111111111111"
	key := []byte(strings.Repeat("p", 48))
	sessions, err := newClientSessionManager(key, nil)
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := sessions.issue(owner)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(Options{ClientSessionKey: key, BlockSyntheticAccountDeletion: true})
	for _, body := range []string{`{"source":"WEB"}`, `{"source":"IOS"}`, `{"source":"ANDROID"}`} {
		request := httptest.NewRequest(http.MethodPost, "/v1/account-deletions", strings.NewReader(body))
		request.AddCookie(cookie)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, request)
		if w.Code != http.StatusServiceUnavailable ||
			!strings.Contains(w.Body.String(), "ACCOUNT_DELETION_EVIDENCE_UNAVAILABLE") ||
			strings.Contains(w.Body.String(), "profile_erased") {
			t.Fatalf("persistent deletion must fail closed: status=%d body=%s", w.Code, w.Body.String())
		}
	}
}
