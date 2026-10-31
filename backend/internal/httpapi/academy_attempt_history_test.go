package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type learnerAttemptStub struct {
	academyStoreStub
	calls int
	actor string
}

func (s *learnerAttemptStub) AcademyLearnerAttempts(actor, enrollment string) ([]academy.Attempt, error) {
	s.calls++
	s.actor = actor
	return []academy.Attempt{}, nil
}

func TestAcademyLearnerAttemptHistoryRequiresSignedSession(t *testing.T) {
	key := []byte(strings.Repeat("a", 48))
	stub := &learnerAttemptStub{}
	handler := New(Options{Academy: stub, ClientSessionKey: key})
	path := "/v1/academy/enrollments/00000000-0000-0000-0000-000000000001/attempts"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	if w.Code != http.StatusUnauthorized || stub.calls != 0 {
		t.Fatalf("unsigned session reached private results: status=%d calls=%d", w.Code, stub.calls)
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
	if w.Code != http.StatusOK || stub.calls != 1 ||
		stub.actor != "00000000-0000-0000-0000-00000000b001" ||
		w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("signed result projection failure: code=%d calls=%d", w.Code, stub.calls)
	}

	// Malformed enrollment IDs must fail before the canonical store sees them.
	req = httptest.NewRequest(http.MethodGet, "/v1/academy/enrollments/invalid/attempts", nil)
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || stub.calls != 1 {
		t.Fatalf("malformed enrollment reached store: code=%d calls=%d", w.Code, stub.calls)
	}
}
