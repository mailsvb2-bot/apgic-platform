package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type academyStoreStub struct {
	catalog []academy.Course
	calls   int
}

func (s *academyStoreStub) AcademyCreateCourse(_ string, _ academy.Course) (academy.Course, error) {
	s.calls++
	return academy.Course{}, academy.ErrDenied
}
func (s *academyStoreStub) AcademyNewVersion(_, _ string) (academy.Course, error) {
	s.calls++
	return academy.Course{}, academy.ErrDenied
}
func (s *academyStoreStub) AcademyAddModule(_, _, _ string, _ academy.Module) (academy.Module, error) {
	s.calls++
	return academy.Module{}, academy.ErrDenied
}
func (s *academyStoreStub) AcademyAddLesson(_, _, _, _ string, _ academy.Lesson) (academy.Lesson, error) {
	s.calls++
	return academy.Lesson{}, academy.ErrDenied
}
func (s *academyStoreStub) AcademyDeclareOriginalRights(_, _, _, _ string) (academy.OriginalRightsDeclaration, error) {
	s.calls++
	return academy.OriginalRightsDeclaration{}, academy.ErrDenied
}
func (s *academyStoreStub) AcademyPublish(_, _, _ string) (academy.Course, error) {
	s.calls++
	return academy.Course{}, academy.ErrDenied
}
func (s *academyStoreStub) AcademyCatalog() ([]academy.Course, error) { return s.catalog, nil }
func (s *academyStoreStub) AcademyEnroll(_, _ string) (academy.Enrollment, error) {
	s.calls++
	return academy.Enrollment{}, academy.ErrEntitlement
}
func (s *academyStoreStub) AcademyLearning(_, _ string) ([]academy.Lesson, error) {
	s.calls++
	return nil, academy.ErrEntitlement
}
func (s *academyStoreStub) AcademyMarkProgress(_, _, _ string) error {
	s.calls++
	return academy.ErrDenied
}
func (s *academyStoreStub) AcademyComplete(_, _ string) (academy.Certificate, error) {
	s.calls++
	return academy.Certificate{}, academy.ErrState
}
func (s *academyStoreStub) AcademyVerify(_ string) (map[string]any, error) {
	return nil, academy.ErrNotFound
}
func (s *academyStoreStub) AcademySubmitAttempt(_, _, _, _ string) (academy.Attempt, error) {
	s.calls++
	return academy.Attempt{}, academy.ErrDenied
}
func (s *academyStoreStub) AcademyEvaluateAttempt(_, _, _ string, _ int) (academy.Attempt, error) {
	s.calls++
	return academy.Attempt{}, academy.ErrDenied
}
func (s *academyStoreStub) AcademyAuthorCourses(_ string) ([]academy.Course, error) {
	s.calls++
	return nil, academy.ErrDenied
}
func (s *academyStoreStub) AcademyOutline(_, _, _ string) (map[string]any, error) {
	s.calls++
	return nil, academy.ErrDenied
}
func (s *academyStoreStub) AcademyMyEnrollments(_ string) ([]academy.Enrollment, error) {
	s.calls++
	return nil, academy.ErrDenied
}
func (s *academyStoreStub) AcademyPendingReviews(_ string) ([]academy.ReviewAttempt, error) {
	s.calls++
	return nil, academy.ErrDenied
}

func TestAcademyUnauthenticatedMutationsDoNotReachStore(t *testing.T) {
	fake := &academyStoreStub{}
	handler := New(Options{Academy: fake, ClientSessionKey: []byte(strings.Repeat("x", 40))})
	cases := []string{
		"/v1/academy/courses",
		"/v1/academy/courses/foreign/versions",
		"/v1/academy/courses/foreign/enroll",
		"/v1/academy/enrollments/foreign/complete",
		"/v1/academy/enrollments/foreign/lessons/secret/complete",
		"/v1/academy/attempts/foreign/evaluate",
	}
	for _, path := range cases {
		t.Run(path, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}")))
			if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), "CLIENT_SESSION_REQUIRED") {
				t.Fatalf("missing session must be denied: code=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
	if fake.calls != 0 {
		t.Fatalf("unauthorized requests reached academy store: %d", fake.calls)
	}
}
func TestAcademyPublicCatalogNeverPublishesRawLessonContent(t *testing.T) {
	fake := &academyStoreStub{catalog: []academy.Course{{
		ID: "course-1", Title: "Course", Description: "Public", State: "PUBLISHED", VersionNo: 1,
	}}}
	handler := New(Options{Academy: fake})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/academy/courses", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "\"title\":\"Course\"") ||
		strings.Contains(w.Body.String(), "body") || strings.Contains(w.Body.String(), "media_asset_ref") {
		t.Fatalf("catalog leaked learning content or failed: %d %s", w.Code, w.Body.String())
	}
}
func TestAcademyErrorsDoNotLeakDatabaseDetail(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{academy.ErrDenied, 403, "ACADEMY_ACCESS_DENIED"},
		{academy.ErrEntitlement, 402, "ACADEMY_ENTITLEMENT_REQUIRED"},
		{academy.ErrState, 409, "ACADEMY_STATE_CONFLICT"},
		{errors.New("select user_private_token from academy_attempts"), 503, "ACADEMY_UNAVAILABLE"},
	} {
		w := httptest.NewRecorder()
		academyHTTPError(w, httptest.NewRequest(http.MethodGet, "/v1/academy", nil), tc.err)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) ||
			strings.Contains(w.Body.String(), "user_private_token") {
			t.Fatalf("unsafe academy error status=%d body=%s", w.Code, w.Body.String())
		}
	}
}

func TestAcademyFailsClosedWhenSignedSessionsAreNotConfigured(t *testing.T) {
	fake := &academyStoreStub{}
	handler := New(Options{Academy: fake})
	for _, request := range []struct{ method, path string }{
		{http.MethodPost, "/v1/academy/courses"},
		{http.MethodPost, "/v1/academy/courses/forged/enroll"},
		{http.MethodGet, "/v1/academy/my-courses"},
		{http.MethodGet, "/v1/academy/my-enrollments"},
		{http.MethodGet, "/v1/academy/review/attempts"},
	} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(request.method, request.path, strings.NewReader("{}")))
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "ACADEMY_SESSION_UNAVAILABLE") {
			t.Fatalf("unsigned Academy principal unexpectedly trusted: %s HTTP %d %s", request.path, w.Code, w.Body.String())
		}
	}
	if fake.calls != 0 {
		t.Fatalf("unsigned requests reached repository: %d", fake.calls)
	}
}

func TestAcademyJSONRejectsAmbiguousMultiObjectRequests(t *testing.T) {
	fake := &academyStoreStub{}
	signingKey := []byte(strings.Repeat("k", 48))
	sessions, err := newClientSessionManager(signingKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := sessions.issue("identity-cryptographically-signed")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(Options{Academy: fake, ClientSessionKey: signingKey})
	for _, body := range []string{
		`{"title":"course"}{"product_id":"forged"}`,
		`{"title":"course"} trailing`,
		`{"title":"course","unknown_role":"owner"}`,
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/academy/courses", strings.NewReader(body))
		req.AddCookie(cookie)
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "ACADEMY_INVALID") {
			t.Fatalf("unsafe Academy JSON accepted: %d %s", w.Code, w.Body.String())
		}
	}
	if fake.calls != 0 {
		t.Fatalf("malformed requests reached repository: %d", fake.calls)
	}
}
