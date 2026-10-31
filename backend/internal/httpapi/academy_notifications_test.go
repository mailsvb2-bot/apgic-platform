package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type academyNotificationStub struct {
	academyStoreStub
	calls int
	actor string
}

func (s *academyNotificationStub) AcademyNotificationPreferences(actor string) (academy.NotificationPreferences, error) {
	s.calls++
	s.actor = actor
	return academy.NotificationPreferences{}, nil
}
func (s *academyNotificationStub) AcademySaveNotificationPreferences(actor string, p academy.NotificationPreferences) (academy.NotificationPreferences, error) {
	s.calls++
	s.actor = actor
	return p, nil
}

func TestAcademyNotificationPreferencesRequireSignedIdentity(t *testing.T) {
	key := []byte(strings.Repeat("z", 48))
	store := &academyNotificationStub{}
	handler := New(Options{Academy: store, ClientSessionKey: key})
	path := "/v1/academy/me/notifications"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	if w.Code != http.StatusUnauthorized || store.calls != 0 {
		t.Fatalf("anonymous preferences request reached store")
	}
	sessions, err := newClientSessionManager(key, nil)
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := sessions.issue("00000000-0000-0000-0000-00000000b001")
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		body := ""
		if method == http.MethodPost {
			body = `{"enabled":false,"learning_updates":false,"assessment_results":false,"certificate_updates":false}`
		}
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.AddCookie(cookie)
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("%s preferences: status=%d body=%s", method, w.Code, w.Body.String())
		}
	}
	if store.calls != 2 || store.actor != "00000000-0000-0000-0000-00000000b001" {
		t.Fatalf("preferences not bound to authenticated identity")
	}
}
