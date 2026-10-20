package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/marketplace"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/specialist"
)

type specialistStoreSpy struct {
	identities []string
	profile    specialist.Profile
}

func (s *specialistStoreSpy) SpecialistWorkspace(_ context.Context, identityID string, from, until time.Time) ([]specialist.WorkspaceEntry, error) {
	s.capture(identityID)
	return []specialist.WorkspaceEntry{{SlotID: "owner-slot", BookingID: "owner-booking", BookingState: "CONFIRMED", StartsAt: from, EndsAt: until}}, nil
}

func (s *specialistStoreSpy) capture(identityID string) {
	s.identities = append(s.identities, identityID)
}

func (s *specialistStoreSpy) UpsertProfile(identityID, displayName, professionCode string) (specialist.Profile, error) {
	s.capture(identityID)
	s.profile = specialist.Profile{
		ID:              "profile-test",
		IdentityID:      identityID,
		DisplayName:     displayName,
		ProfessionCode:  professionCode,
		ProfileComplete: true,
		ReviewState:     marketplace.ReviewPending,
		Capabilities:    []specialist.Capability{},
		Evidence:        []specialist.Evidence{},
		PublishedTopics: []string{},
	}
	return s.profile, nil
}

func (s *specialistStoreSpy) Profile(identityID string) (specialist.Profile, error) {
	s.capture(identityID)
	return s.profile, nil
}

func (s *specialistStoreSpy) DeclareCapability(identityID, topicID string) (specialist.Profile, error) {
	s.capture(identityID)
	return s.profile, nil
}

func (s *specialistStoreSpy) SubmitEvidence(identityID, topicID, kind, reference string) (specialist.Profile, error) {
	s.capture(identityID)
	return s.profile, nil
}

func (s *specialistStoreSpy) Publish(identityID, topicID string) (specialist.PublishResult, error) {
	s.capture(identityID)
	return specialist.PublishResult{
		Allowed:       false,
		ReasonCodes:   []string{marketplace.ReasonReviewIncomplete},
		PolicyVersion: "qualification-v1",
	}, nil
}

func (s *specialistStoreSpy) Unpublish(identityID, topicID string) (specialist.PublishResult, error) {
	s.capture(identityID)
	return specialist.PublishResult{
		Allowed:       true,
		ReasonCodes:   []string{marketplace.ReasonUnpublished},
		PolicyVersion: "qualification-v1",
	}, nil
}

func TestSpecialistHTTPKeepsOneSignedIdentityAndExposesNoReviewMutation(t *testing.T) {
	store := &specialistStoreSpy{}
	handler := New(Options{
		Specialists:      store,
		ClientSessionKey: []byte("specialist-http-test-key-000000000000"),
	})

	create := httptest.NewRequest(http.MethodPut, "/v1/specialist/profile",
		strings.NewReader(`{"display_name":"Анна Тестова","profession_code":"PSYCHOLOGIST"}`))
	create.Header.Set("content-type", "application/json")
	createRecorder := httptest.NewRecorder()
	handler.ServeHTTP(createRecorder, create)
	if createRecorder.Code != http.StatusOK {
		t.Fatalf("profile upsert status=%d body=%s", createRecorder.Code, createRecorder.Body.String())
	}
	cookies := createRecorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != clientSessionCookieName {
		t.Fatalf("specialist bootstrap did not issue signed session: %#v", cookies)
	}
	if len(store.identities) != 1 || store.identities[0] == "" {
		t.Fatalf("specialist bootstrap identity = %#v", store.identities)
	}
	identityID := store.identities[0]

	read := httptest.NewRequest(http.MethodGet, "/v1/specialist/profile", nil)
	read.AddCookie(cookies[0])
	readRecorder := httptest.NewRecorder()
	handler.ServeHTTP(readRecorder, read)
	if readRecorder.Code != http.StatusOK {
		t.Fatalf("profile read status=%d body=%s", readRecorder.Code, readRecorder.Body.String())
	}
	if len(store.identities) != 2 || store.identities[1] != identityID {
		t.Fatalf("specialist profile switched identity: %#v", store.identities)
	}

	publish := httptest.NewRequest(http.MethodPost, "/v1/specialist/publish",
		strings.NewReader(`{"topic_id":"anxiety"}`))
	publish.Header.Set("content-type", "application/json")
	publish.AddCookie(cookies[0])
	publishRecorder := httptest.NewRecorder()
	handler.ServeHTTP(publishRecorder, publish)
	if publishRecorder.Code != http.StatusConflict {
		t.Fatalf("unreviewed publish status=%d body=%s", publishRecorder.Code, publishRecorder.Body.String())
	}
	if !strings.Contains(publishRecorder.Body.String(), marketplace.ReasonReviewIncomplete) {
		t.Fatalf("unreviewed publish lost reason code: %s", publishRecorder.Body.String())
	}

	review := httptest.NewRequest(http.MethodPost, "/v1/specialist/review",
		strings.NewReader(`{"approved":true}`))
	review.AddCookie(cookies[0])
	reviewRecorder := httptest.NewRecorder()
	handler.ServeHTTP(reviewRecorder, review)
	if reviewRecorder.Code != http.StatusNotFound {
		t.Fatalf("public review mutation unexpectedly exists: status=%d body=%s", reviewRecorder.Code, reviewRecorder.Body.String())
	}
}

func TestSpecialistWorkspaceRejectsAnonymousAndUsesSignedOwner(t *testing.T) {
	store := &specialistStoreSpy{}
	handler := New(Options{
		Specialists:      store,
		ClientSessionKey: []byte("specialist-http-test-key-000000000000"),
	})
	path := "/v1/specialist/workspace?from=2026-10-01T00:00:00Z&until=2026-11-01T00:00:00Z"
	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, path, nil))
	if unauthenticated.Code == http.StatusOK || len(store.identities) != 0 {
		t.Fatalf("anonymous workspace accessible: status=%d", unauthenticated.Code)
	}
	create := httptest.NewRecorder()
	handler.ServeHTTP(create, httptest.NewRequest(http.MethodPut, "/v1/specialist/profile",
		strings.NewReader(`{"display_name":"Анна Тестова","profession_code":"PSYCHOLOGIST"}`)))
	if create.Code != http.StatusOK {
		t.Fatalf("create specialist: %d %s", create.Code, create.Body.String())
	}
	cookies := create.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("missing signed specialist session")
	}
	owner := store.identities[0]
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(cookies[0])
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "owner-booking") {
		t.Fatalf("authenticated workspace status=%d body=%s", response.Code, response.Body.String())
	}
	if len(store.identities) != 2 || store.identities[1] != owner {
		t.Fatalf("workspace did not use signed identity: %#v", store.identities)
	}
	invalid := httptest.NewRequest(http.MethodGet,
		"/v1/specialist/workspace?from=2026-10-01T00:00:00Z&until=2029-10-01T00:00:00Z", nil)
	invalid.AddCookie(cookies[0])
	rejected := httptest.NewRecorder()
	handler.ServeHTTP(rejected, invalid)
	if rejected.Code != http.StatusBadRequest {
		t.Fatalf("unbounded calendar range allowed: %d", rejected.Code)
	}
}
