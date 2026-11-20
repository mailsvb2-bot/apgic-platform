package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type academySessionAuthorStub struct{ academyStore }

func (*academySessionAuthorStub) AcademyAuthorCourses(string) ([]academy.Course, error) {
	return []academy.Course{}, nil
}

func TestAcademyAuthorSessionExplicitBootstrap(t *testing.T) {
	key := []byte(strings.Repeat("s", 40))
	handler := New(Options{ClientSessionKey: key, Academy: &academySessionAuthorStub{}})
	noSession := httptest.NewRecorder()
	handler.ServeHTTP(noSession, httptest.NewRequest(http.MethodGet, "/v1/academy/my-courses", nil))
	if noSession.Code != http.StatusUnauthorized {
		t.Fatalf("missing session should deny private courses, status=%d", noSession.Code)
	}
	request := httptest.NewRequest(http.MethodPost, "https://apgic.ru/v1/academy/session/start", strings.NewReader("{}"))
	request.Header.Set("Origin", "https://apgic.ru")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, request)
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), `"new_identity":true`) {
		t.Fatalf("explicit bootstrap status=%d body=%s", resp.Code, resp.Body.String())
	}
	cookies := resp.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != clientSessionCookieName {
		t.Fatalf("expected canonical signed cookie, got %v", cookies)
	}
	owner := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/academy/my-courses", nil)
	r.AddCookie(cookies[0])
	handler.ServeHTTP(owner, r)
	if owner.Code != http.StatusOK {
		// A valid session reaches the existing author-owned course list.
		t.Fatalf("signed session was not accepted, status=%d body=%s", owner.Code, owner.Body.String())
	}
	reuse := httptest.NewRequest(http.MethodPost, "https://apgic.ru/v1/academy/session/start", strings.NewReader("{}"))
	reuse.Header.Set("Origin", "https://apgic.ru")
	reuse.AddCookie(cookies[0])
	reused := httptest.NewRecorder()
	handler.ServeHTTP(reused, reuse)
	if reused.Code != 200 || !strings.Contains(reused.Body.String(), `"new_identity":false`) || len(reused.Result().Cookies()) != 0 {
		t.Fatalf("bootstrap replaced a valid owner session: %d %s", reused.Code, reused.Body.String())
	}
}

func TestAcademyAuthorSessionRejectsCrossOriginAndInvalidCookie(t *testing.T) {
	handler := New(Options{ClientSessionKey: []byte(strings.Repeat("s", 40))})
	for _, origin := range []string{"https://attacker.example", "http://apgic.ru"} {
		r := httptest.NewRequest(http.MethodPost, "https://apgic.ru/v1/academy/session/start", nil)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden || len(w.Result().Cookies()) != 0 {
			t.Fatalf("origin %s accepted: %d %s", origin, w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest(http.MethodPost, "https://apgic.ru/v1/academy/session/start", nil)
	r.Header.Set("Origin", "https://apgic.ru")
	r.AddCookie(&http.Cookie{Name: clientSessionCookieName, Value: "tampered"})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized || len(w.Result().Cookies()) != 0 {
		t.Fatalf("invalid cookie overwritten: %d %s", w.Code, w.Body.String())
	}
}
