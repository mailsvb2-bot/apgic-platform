package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type academyReportStub struct {
	academyStoreStub
	calls  int
	author string
}

func (s *academyReportStub) AcademyAuthorReport(author string) (academy.AuthorReport, error) {
	s.calls++
	s.author = author
	return academy.AuthorReport{
		Courses:       []academy.CourseReport{{CourseID: "owned-course", Enrollments: 1}},
		GrossCaptures: []academy.CapturedGrossEvidence{},
		RevenueState:  academy.RevenueGrossOnly,
	}, nil
}

func TestAcademyAuthorReportRequiresSignedOwner(t *testing.T) {
	key := []byte(strings.Repeat("r", 48))
	fake := &academyReportStub{}
	handler := New(Options{Academy: fake, ClientSessionKey: key})
	path := "/v1/academy/author/report"

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	if w.Code != http.StatusUnauthorized || fake.calls != 0 {
		t.Fatalf("unauthorized report reached store: HTTP %d calls=%d", w.Code, fake.calls)
	}

	sessions, err := newClientSessionManager(key, nil)
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := sessions.issue("00000000-0000-0000-0000-00000000b001")
	if err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(cookie)
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK || fake.calls != 1 || fake.author != "00000000-0000-0000-0000-00000000b001" {
		t.Fatalf("signed author not bound: HTTP %d calls=%d author=%s", w.Code, fake.calls, fake.author)
	}
	if !strings.Contains(w.Body.String(), academy.RevenueGrossOnly) ||
		strings.Contains(w.Body.String(), "response_text") ||
		w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("unsafe report projection: %s", w.Body.String())
	}
}
