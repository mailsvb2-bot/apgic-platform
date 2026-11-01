package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type academyCertificatesStub struct {
	academyStoreStub
	calls int
	actor string
}

func (s *academyCertificatesStub) AcademyMyCertificates(actor string) ([]academy.LearnerCertificate, error) {
	s.calls++
	s.actor = actor
	return []academy.LearnerCertificate{{
		ID:                     "owned-certificate",
		State:                  "ACTIVE",
		LearningAccessState:    "REVOKED",
		VerificationURL:        "/v1/academy/certificates/verify/opaque-test-token",
		EvidenceCandidateState: academy.EvidenceCandidateStatus,
	}}, nil
}

func TestAcademyMyCertificatesRequiresSignedLearner(t *testing.T) {
	key := []byte(strings.Repeat("c", 48))
	store := &academyCertificatesStub{}
	handler := New(Options{Academy: store, ClientSessionKey: key})
	path := "/v1/academy/me/certificates"

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	if w.Code != http.StatusUnauthorized || store.calls != 0 {
		t.Fatalf("unsigned certificate history reached owner: status=%d calls=%d", w.Code, store.calls)
	}
	sessions, err := newClientSessionManager(key, nil)
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := sessions.issue("00000000-0000-0000-0000-00000000b003")
	if err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(cookie)
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK || store.calls != 1 || store.actor != "00000000-0000-0000-0000-00000000b003" {
		t.Fatalf("signed learner not bound: status=%d calls=%d actor=%s", w.Code, store.calls, store.actor)
	}
	if w.Header().Get("Cache-Control") != "private, no-store" ||
		!strings.Contains(w.Body.String(), "learning_access_state") ||
		!strings.Contains(w.Body.String(), "REVOKED") ||
		strings.Contains(w.Body.String(), "response_text") {
		t.Fatalf("certificate readback disclosed private data or lost state: %s", w.Body.String())
	}
}
