package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type cohortStoreStub struct {
	*academyStoreStub
	cohortCalls int
	actor       string
}

func (s *cohortStoreStub) AcademyListCohorts(actor string) ([]academy.Cohort, error) {
	s.cohortCalls++
	s.actor = actor
	return []academy.Cohort{}, nil
}
func (s *cohortStoreStub) AcademyCreateCohort(actor, versionID, label string, capacity int, startsAt, closesAt *time.Time) (academy.Cohort, error) {
	s.cohortCalls++
	s.actor = actor
	return academy.Cohort{ID: "00000000-0000-0000-0000-000000000003", VersionID: versionID, Label: label, Capacity: capacity, StartsAt: startsAt, EnrollmentClosesAt: closesAt}, nil
}
func (s *cohortStoreStub) AcademyJoinCohort(actor, cohortID, enrollmentID string) error {
	s.cohortCalls++
	s.actor = actor
	return nil
}

func TestAcademyCohortsRequireSignedIdentityAndValidatedIDs(t *testing.T) {
	const identity = "00000000-0000-0000-0000-00000000b001"
	const versionID = "00000000-0000-0000-0000-000000000002"
	const cohortID = "00000000-0000-0000-0000-000000000003"
	const enrollmentID = "00000000-0000-0000-0000-000000000004"
	key := []byte(strings.Repeat("c", 48))
	sessions, err := newClientSessionManager(key, nil)
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := sessions.issue(identity)
	if err != nil {
		t.Fatal(err)
	}
	store := &cohortStoreStub{academyStoreStub: &academyStoreStub{}}
	handler := New(Options{Academy: store, ClientSessionKey: key})
	cases := []struct {
		method, path, body string
		status             int
	}{
		{http.MethodGet, "/v1/academy/author/cohorts", "", http.StatusOK},
		{http.MethodPost, "/v1/academy/versions/" + versionID + "/cohorts", `{"label":"First cohort","capacity":2,"starts_at":"2030-11-01T12:00:00Z"}`, http.StatusCreated},
		{http.MethodPost, "/v1/academy/cohorts/" + cohortID + "/join", `{"enrollment_id":"` + enrollmentID + `"}`, http.StatusOK},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("unsigned cohort request reached store: %s %d %s", tc.path, w.Code, w.Body.String())
		}
	}
	if store.cohortCalls != 0 {
		t.Fatalf("unsigned requests reached cohort store: %d", store.cohortCalls)
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.AddCookie(cookie)
		handler.ServeHTTP(w, req)
		if w.Code != tc.status || store.actor != identity {
			t.Fatalf("signed cohort request failed: %s %d %s", tc.path, w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("private cohort response cacheable: %s", tc.path)
		}
	}
	if store.cohortCalls != len(cases) {
		t.Fatalf("expected %d canonical cohort calls, got %d", len(cases), store.cohortCalls)
	}
	for _, tc := range []struct {
		path, body string
	}{
		{"/v1/academy/cohorts/invalid/join", `{"enrollment_id":"` + enrollmentID + `"}`},
		{"/v1/academy/cohorts/" + cohortID + "/join", `{"enrollment_id":"invalid"}`},
		{"/v1/academy/versions/invalid/cohorts", `{"label":"No","capacity":1}`},
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
		req.AddCookie(cookie)
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("malformed cohort identifiers accepted: %s %d %s", tc.path, w.Code, w.Body.String())
		}
	}
	if store.cohortCalls != len(cases) {
		t.Fatalf("invalid identifiers reached cohort store: %d", store.cohortCalls)
	}
}
