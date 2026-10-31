package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type academyScheduleStub struct {
	academyStoreStub
	calls  int
	actor  string
	lesson string
}

func (s *academyScheduleStub) AcademySetLessonSchedule(actor, lesson string, start, end *time.Time) error {
	s.calls++
	s.actor = actor
	s.lesson = lesson
	if start == nil {
		return academy.ErrInvalid
	}
	return nil
}

func (s *academyScheduleStub) AcademySetAssessmentLimits(actor, lesson string, max int, deadline *time.Time) error {
	s.calls++
	s.actor = actor
	s.lesson = lesson
	if max < 1 {
		return academy.ErrInvalid
	}
	return nil
}

func TestAcademyScheduleRequiresSignedAuthorAndValidLesson(t *testing.T) {
	key := []byte(strings.Repeat("s", 48))
	store := &academyScheduleStub{}
	handler := New(Options{Academy: store, ClientSessionKey: key})
	path := "/v1/academy/lessons/00000000-0000-0000-0000-000000000011/schedule"
	body := `{"release_at":"2027-01-01T10:00:00Z"}`
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	if w.Code != http.StatusUnauthorized || store.calls != 0 {
		t.Fatalf("anonymous schedule write: status=%d calls=%d", w.Code, store.calls)
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
		body string
		want int
	}{
		{path: "/v1/academy/lessons/not-a-uuid/schedule", body: body, want: http.StatusBadRequest},
		{path: path, body: body, want: http.StatusOK},
		{path: "/v1/academy/lessons/00000000-0000-0000-0000-000000000011/assessment-limits", body: `{"max_attempts":2}`, want: http.StatusOK},
	} {
		req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
		req.AddCookie(cookie)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Fatalf("path=%s: got=%d want=%d: %s", tc.path, w.Code, tc.want, w.Body.String())
		}
	}
	if store.calls != 2 || store.actor != "00000000-0000-0000-0000-00000000b001" ||
		store.lesson != "00000000-0000-0000-0000-000000000011" {
		t.Fatalf("invalid canonical schedule owner routing")
	}
}
