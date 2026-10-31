package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type learnerCohortStub struct {
	academyStoreStub
	calls int
	actor string
}

func (s *learnerCohortStub) AcademyLearnerCohorts(actor, enrollmentID string) ([]academy.Cohort, error) {
	s.calls++
	s.actor = actor
	return []academy.Cohort{{ID: "00000000-0000-0000-0000-000000000012", IsMember: true}}, nil
}

func TestAcademyLearnerCohortsIdentityBoundary(t *testing.T) {
	key := []byte(strings.Repeat("q", 48))
	store := &learnerCohortStub{}
	handler := New(Options{Academy: store, ClientSessionKey: key})
	path := "/v1/academy/enrollments/00000000-0000-0000-0000-000000000011/cohorts"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	if w.Code != http.StatusUnauthorized || store.calls != 0 {
		t.Fatalf("anonymous cohort data request: code=%d calls=%d", w.Code, store.calls)
	}
	sessions, err := newClientSessionManager(key, nil)
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := sessions.issue("00000000-0000-0000-0000-00000000b001")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path string
		code int
	}{
		{path: "/v1/academy/enrollments/invalid/cohorts", code: http.StatusBadRequest},
		{path: path, code: http.StatusOK},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.AddCookie(cookie)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != tc.code {
			t.Fatalf("path=%s got=%d want=%d: %s", tc.path, w.Code, tc.code, w.Body.String())
		}
		if tc.code == http.StatusOK && !strings.Contains(w.Body.String(), `"is_member":true`) {
			t.Fatalf("member status missing: %s", w.Body.String())
		}
	}
	if store.calls != 1 || store.actor != "00000000-0000-0000-0000-00000000b001" {
		t.Fatalf("learner cohort query not bound to signed identity")
	}
}
