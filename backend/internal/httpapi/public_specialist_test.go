package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/specialist"
)

type publicSpecialistSpy struct {
	*specialistStoreSpy
	details  specialist.PublicDetails
	savedFor string
	found    bool
}

func (s *publicSpecialistSpy) PublicProfile(_ context.Context, id string) (specialist.PublicProfile, error) {
	if !s.found || id != "00000000-0000-0000-0000-000000000001" {
		return specialist.PublicProfile{}, specialist.ErrPublicProfileNotFound
	}
	return specialist.PublicProfile{ID: id, DisplayName: "Специалист", ProfessionCode: "PSYCHOLOGIST",
		PublishedTopics: []string{"anxiety"}, Languages: []string{"ru"}}, nil
}
func (s *publicSpecialistSpy) PublicProfiles(_ context.Context, topic string, limit, offset int) ([]specialist.PublicProfile, error) {
	if topic != "" && topic != "anxiety" || offset > 0 {
		return []specialist.PublicProfile{}, nil
	}
	p, _ := s.PublicProfile(context.Background(), "00000000-0000-0000-0000-000000000001")
	return []specialist.PublicProfile{p}, nil
}
func (s *publicSpecialistSpy) PublicDetails(_ context.Context, identity string) (specialist.PublicDetails, error) {
	s.savedFor = identity
	if s.profile.ID == "" {
		return specialist.PublicDetails{}, specialist.ErrProfileNotFound
	}
	return s.details, nil
}
func (s *publicSpecialistSpy) SavePublicDetails(_ context.Context, identity string, p specialist.PublicDetails) (specialist.PublicDetails, error) {
	s.savedFor = identity
	normalized, err := specialist.NormalizePublicDetails(p)
	if err != nil {
		return specialist.PublicDetails{}, err
	}
	s.details = normalized
	return normalized, nil
}
func TestPublicDirectoryHidesPrivateEvidenceAndRequiresPublishedGate(t *testing.T) {
	spy := &publicSpecialistSpy{specialistStoreSpy: &specialistStoreSpy{}, found: true}
	handler := New(Options{Specialists: spy, ClientSessionKey: []byte("public-specialist-test-key-00000000")})
	list := httptest.NewRecorder()
	handler.ServeHTTP(list, httptest.NewRequest("GET", "/v1/specialists?topic=anxiety", nil))
	if list.Code != 200 {
		t.Fatalf("directory %d: %s", list.Code, list.Body.String())
	}
	if strings.Contains(list.Body.String(), "identity_id") || strings.Contains(list.Body.String(), "evidence_ref") {
		t.Fatalf("private specialist information exposed: %s", list.Body.String())
	}
	var data struct {
		Profiles []specialist.PublicProfile `json:"profiles"`
		HasMore  bool                       `json:"has_more"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &data); err != nil || len(data.Profiles) != 1 {
		t.Fatalf("invalid directory: %s, %v", list.Body.String(), err)
	}
	for _, path := range []string{"/v1/specialists?topic=unsupported", "/v1/specialists?limit=50", "/v1/specialists?offset=-1"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 400 {
			t.Fatalf("%s = %d, want 400", path, w.Code)
		}
	}
	get := httptest.NewRecorder()
	handler.ServeHTTP(get, httptest.NewRequest("GET", "/v1/specialists/00000000-0000-0000-0000-000000000001", nil))
	if get.Code != 200 {
		t.Fatalf("public profile = %d", get.Code)
	}
	spy.found = false
	hidden := httptest.NewRecorder()
	handler.ServeHTTP(hidden, httptest.NewRequest("GET", "/v1/specialists/00000000-0000-0000-0000-000000000001", nil))
	if hidden.Code != 404 {
		t.Fatalf("unpublished profile is visible: %d", hidden.Code)
	}
}
func TestPublicDetailsWriteRequiresSignedOwnerSession(t *testing.T) {
	spy := &publicSpecialistSpy{specialistStoreSpy: &specialistStoreSpy{}}
	handler := New(Options{Specialists: spy, ClientSessionKey: []byte("public-specialist-test-key-00000000")})
	reqBody := `{"headline":"Консультирую по тревоге","about":"Только онлайн","experience_years":3,"languages":["ru"],"limitations":["Не работаю с детьми"]}`
	unauth := httptest.NewRecorder()
	handler.ServeHTTP(unauth, httptest.NewRequest("PUT", "/v1/specialist/public-details", strings.NewReader(reqBody)))
	if unauth.Code != 401 {
		t.Fatalf("anonymous edit = %d", unauth.Code)
	}
	bootstrap := httptest.NewRecorder()
	handler.ServeHTTP(bootstrap, httptest.NewRequest("PUT", "/v1/specialist/profile", strings.NewReader(`{"display_name":"Специалист","profession_code":"PSYCHOLOGIST"}`)))
	if bootstrap.Code != 200 {
		t.Fatalf("bootstrap = %d: %s", bootstrap.Code, bootstrap.Body.String())
	}
	cookie := bootstrap.Result().Cookies()[0]
	change := httptest.NewRequest("PUT", "/v1/specialist/public-details", strings.NewReader(reqBody))
	change.AddCookie(cookie)
	ok := httptest.NewRecorder()
	handler.ServeHTTP(ok, change)
	if ok.Code != 200 || !strings.Contains(ok.Body.String(), "review_required") {
		t.Fatalf("owner edit = %d, %s", ok.Code, ok.Body.String())
	}
	if spy.savedFor != spy.profile.IdentityID {
		t.Fatal("owner identity switched")
	}
	bad := httptest.NewRequest("PUT", "/v1/specialist/public-details", strings.NewReader(`{"headline":"test","experience_years":100}`))
	bad.AddCookie(cookie)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, bad)
	if w.Code != 400 {
		t.Fatalf("invalid detail edit = %d", w.Code)
	}
	inject := httptest.NewRequest("PUT", "/v1/specialist/public-details", strings.NewReader(`{"headline":"test","review_state":"APPROVED"}`))
	inject.AddCookie(cookie)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, inject)
	if w.Code != 400 {
		t.Fatalf("caller attempted review override = %d", w.Code)
	}
}

type publicOffersSpy struct {
	*publicSpecialistSpy
	offers []specialist.PublicBookableOffer
}

func (s *publicOffersSpy) PublicOffers(_ context.Context, specialistID string) ([]specialist.PublicBookableOffer, error) {
	if !s.found || specialistID != "00000000-0000-0000-0000-000000000001" {
		return nil, specialist.ErrPublicProfileNotFound
	}
	return s.offers, nil
}
func TestPublicBookableOfferEndpointFailsClosedAndNeverLeaksPrivateTruth(t *testing.T) {
	spy := &publicOffersSpy{
		publicSpecialistSpy: &publicSpecialistSpy{specialistStoreSpy: &specialistStoreSpy{}, found: true},
		offers: []specialist.PublicBookableOffer{{
			ID: "offer-1", TopicID: "anxiety", PriceMinor: 250000, Currency: "RUB",
			Slots: []specialist.PublicBookableSlot{},
		}},
	}
	handler := New(Options{Specialists: spy, ClientSessionKey: []byte("public-offers-test-key-00000000")})
	path := "/v1/specialists/00000000-0000-0000-0000-000000000001/offers"
	ok := httptest.NewRecorder()
	handler.ServeHTTP(ok, httptest.NewRequest("GET", path, nil))
	if ok.Code != 200 {
		t.Fatalf("public offer status %d: %s", ok.Code, ok.Body.String())
	}
	if !strings.Contains(ok.Body.String(), "250000") || strings.Contains(ok.Body.String(), "identity_id") ||
		strings.Contains(ok.Body.String(), "evidence_ref") || strings.Contains(ok.Body.String(), "product_id") {
		t.Fatalf("public offers leaked private data or lost real price: %s", ok.Body.String())
	}
	if ok.Result().Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("public response was cacheable")
	}
	spy.found = false
	revoked := httptest.NewRecorder()
	handler.ServeHTTP(revoked, httptest.NewRequest("GET", path, nil))
	if revoked.Code != 404 {
		t.Fatalf("withdrawn specialist still had public offers: %d", revoked.Code)
	}
}
