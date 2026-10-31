package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type academyAppealStub struct {
	academyStoreStub
	calls int
	actor string
}

func (s *academyAppealStub) AcademySubmitAppeal(actor, attempt, reason string) (academy.Appeal, error) {
	s.calls++
	s.actor = actor
	return academy.Appeal{ID: attempt, AttemptID: attempt, Reason: reason, State: "SUBMITTED"}, nil
}

func (s *academyAppealStub) AcademyMyAppeals(actor string) ([]academy.Appeal, error) {
	s.calls++
	s.actor = actor
	return []academy.Appeal{}, nil
}

func TestAcademyAppealsRequireSignedLearner(t *testing.T) {
	key := []byte(strings.Repeat("y", 48))
	store := &academyAppealStub{}
	handler := New(Options{Academy: store, ClientSessionKey: key})
	path := "/v1/academy/attempts/00000000-0000-0000-0000-000000000011/appeals"
	body := `{"reason":"Оценка выставлена ошибочно, прошу пересмотр."}`
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	if w.Code != http.StatusUnauthorized || store.calls != 0 {
		t.Fatalf("anonymous appeal reached store: %d", w.Code)
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
		method string
		path   string
		body   string
		status int
	}{
		{method: http.MethodPost, path: "/v1/academy/attempts/wrong/appeals", body: body, status: http.StatusBadRequest},
		{method: http.MethodPost, path: path, body: body, status: http.StatusCreated},
		{method: http.MethodGet, path: "/v1/academy/me/appeals", status: http.StatusOK},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.AddCookie(cookie)
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("unexpected %s %s status %d: %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
	if store.calls != 2 || store.actor != "00000000-0000-0000-0000-00000000b001" {
		t.Fatalf("signed appeal was not bound to learner")
	}
}
