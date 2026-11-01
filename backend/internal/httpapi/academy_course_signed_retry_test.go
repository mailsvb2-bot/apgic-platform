package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// The real PostgreSQL retry/uniqueness proof lives in runtimepostgres.
// This verifies that the public HTTP entrypoint binds every retry to the
// signed Identity rather than a caller-supplied author_id.
type academySignedCourseStore struct {
	academyStoreStub
	allowedActor string
	calls        int
}

func (s *academySignedCourseStore) AcademyCreateCourse(actor string, input academy.Course) (academy.Course, error) {
	s.calls++
	if actor != s.allowedActor {
		return academy.Course{}, academy.ErrDenied
	}
	if input.ProductID != "00000000-0000-0000-0000-00000000c001" {
		return academy.Course{}, academy.ErrInvalid
	}
	return academy.Course{
		ID: "00000000-0000-0000-0000-00000000c002",
		ProductID: input.ProductID,
		AuthorID: actor,
		Title: input.Title,
		Description: input.Description,
		State: "DRAFT",
		VersionID: "00000000-0000-0000-0000-00000000c003",
		VersionNo: 1,
	}, nil
}

func TestAcademyCourseCreateSignedSessionRetryAndForeignIdentity(t *testing.T) {
	key := []byte(strings.Repeat("c", 48))
	const author = "00000000-0000-0000-0000-00000000a001"
	const foreign = "00000000-0000-0000-0000-00000000a002"
	store := &academySignedCourseStore{allowedActor: author}
	handler := New(Options{Academy: store, ClientSessionKey: key})
	sessions, err := newClientSessionManager(key, nil)
	if err != nil {
		t.Fatal(err)
	}
	authorCookie, err := sessions.issue(author)
	if err != nil {
		t.Fatal(err)
	}
	foreignCookie, err := sessions.issue(foreign)
	if err != nil {
		t.Fatal(err)
	}
	const path = "/v1/academy/courses"
	const body = `{"product_id":"00000000-0000-0000-0000-00000000c001","title":"Signed course","description":"Course retry","author_id":"00000000-0000-0000-0000-00000000a002"}`
	request := func(cookie *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}
	if w := request(nil); w.Code != http.StatusUnauthorized || store.calls != 0 {
		t.Fatalf("anonymous create must fail before store: status=%d calls=%d body=%s", w.Code, store.calls, w.Body.String())
	}
	for i := 0; i < 2; i++ {
		w := request(authorCookie)
		if w.Code != http.StatusCreated ||
			!strings.Contains(w.Body.String(), `"id":"00000000-0000-0000-0000-00000000c002"`) ||
			!strings.Contains(w.Body.String(), `"author_id":"`+author+`"`) ||
			strings.Contains(w.Body.String(), `"author_id":"`+foreign+`"`) {
			t.Fatalf("signed retry %d: status=%d body=%s", i, w.Code, w.Body.String())
		}
	}
	if w := request(foreignCookie); w.Code != http.StatusForbidden ||
		!strings.Contains(w.Body.String(), "ACADEMY_ACCESS_DENIED") {
		t.Fatalf("foreign identity must be denied: status=%d body=%s", w.Code, w.Body.String())
	}
	if store.calls != 3 {
		t.Fatalf("expected exactly 2 signed retries and 1 foreign denial, got %d", store.calls)
	}
}
