package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAcademyRejectMalformedResourceIDBeforeStore(t *testing.T) {
	key := []byte(strings.Repeat("a", 48))
	sessions, err := newClientSessionManager(key, nil)
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := sessions.issue("00000000-0000-0000-0000-00000000b001")
	if err != nil {
		t.Fatal(err)
	}
	// The existing stub must not be called: a malformed UUID is a client error,
	// not an unhandled PostgreSQL UUID cast error or a resource lookup.
	handler := New(Options{Academy: &academyStoreStub{}, ClientSessionKey: key})
	for _, path := range []string{
		"/v1/academy/enrollments/not-a-uuid/learn",
		"/v1/academy/courses/not-a-uuid/versions/00000000-0000-0000-0000-000000000001/outline",
		"/v1/academy/enrollments/00000000-0000-0000-0000-000000000001/lessons/not-a-uuid/complete",
	} {
		t.Run(path, func(t *testing.T) {
			method := http.MethodGet
			if strings.HasSuffix(path, "/complete") {
				method = http.MethodPost
			}
			req := httptest.NewRequest(method, path, nil)
			req.AddCookie(cookie)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "ACADEMY_INVALID") {
				t.Fatalf("expected invalid ID rejection: status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}
