package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type academyExportStub struct {
	academyStoreStub
	calls int
	actor string
	attemptCalls int
}

func (s *academyExportStub) AcademyMyEnrollments(actor string) ([]academy.Enrollment, error) {
	s.calls++
	s.actor = actor
	return []academy.Enrollment{{ID: "00000000-0000-0000-0000-000000000022"}}, nil
}
func (s *academyExportStub) AcademyLearnerAttempts(actor, enrollmentID string) ([]academy.Attempt, error) {
	s.attemptCalls++
	return []academy.Attempt{{ID: "00000000-0000-0000-0000-000000000033", EnrollmentID: enrollmentID, Result: "PASSED"}}, nil
}
func TestAcademyHistoryExportSignedIdentity(t *testing.T) {
	key := []byte(strings.Repeat("x", 48))
	store := &academyExportStub{}
	handler := New(Options{Academy: store, ClientSessionKey: key})
	path := "/v1/academy/me/history/export"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	if w.Code != http.StatusUnauthorized || store.calls != 0 {
		t.Fatalf("unauthenticated export allowed: status=%d calls=%d", w.Code, store.calls)
	}
	sessions, err := newClientSessionManager(key, nil)
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := sessions.issue("00000000-0000-0000-0000-00000000b001")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK || store.calls != 1 || store.attemptCalls != 1 || store.actor != "00000000-0000-0000-0000-00000000b001" {
		t.Fatalf("signed export failed: status=%d calls=%d", w.Code, store.calls)
	}
	if w.Header().Get("Cache-Control") != "private, no-store" ||
		!strings.Contains(w.Header().Get("Content-Disposition"), "attachment;") ||
		!strings.Contains(w.Body.String(), "academy-learning-export-v1") ||
		!strings.Contains(w.Body.String(), "\"PASSED\"") ||
		strings.Contains(w.Body.String(), "response_text") {
		t.Fatalf("unsafe export response: %s", w.Body.String())
	}
}
