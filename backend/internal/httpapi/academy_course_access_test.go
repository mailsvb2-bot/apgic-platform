package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type academyCourseAccessStub struct {
	academyStoreStub
	calls int
	actor string
}

func (s *academyCourseAccessStub) AcademyMyCourseAccess(actor string) ([]string, error) {
	s.calls++
	s.actor = actor
	return []string{"00000000-0000-0000-0000-00000000c001"}, nil
}

func TestAcademyCourseAccessRequiresSignedLearner(t *testing.T) {
	key := []byte(strings.Repeat("a", 48))
	store := &academyCourseAccessStub{}
	handler := New(Options{Academy: store, ClientSessionKey: key})
	path := "/v1/academy/me/course-access"

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	if w.Code != http.StatusUnauthorized || store.calls != 0 {
		t.Fatalf("anonymous access read reached canonical store: %d calls=%d", w.Code, store.calls)
	}
	sessions, err := newClientSessionManager(key, nil)
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := sessions.issue("00000000-0000-0000-0000-00000000b003")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "private, no-store" ||
		!strings.Contains(w.Body.String(), "00000000-0000-0000-0000-00000000c001") ||
		store.actor != "00000000-0000-0000-0000-00000000b003" || store.calls != 1 {
		t.Fatalf("signed learner access readback failed: %d %s", w.Code, w.Body.String())
	}
}
