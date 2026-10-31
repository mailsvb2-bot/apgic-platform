package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type academyAuthorProductStub struct {
	academyStoreStub
	calls int
	actor string
}

func (s *academyAuthorProductStub) AcademyAuthorProducts(actor string) ([]academy.AuthorProduct, error) {
	s.calls++
	s.actor = actor
	return []academy.AuthorProduct{{ID: "00000000-0000-0000-0000-000000000011", Name: "Published learning product"}}, nil
}

func TestAcademyAuthorProductsRequireSignedIdentity(t *testing.T) {
	key := []byte(strings.Repeat("p", 48))
	store := &academyAuthorProductStub{}
	handler := New(Options{Academy: store, ClientSessionKey: key})
	path := "/v1/academy/author/eligible-products"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	if w.Code != http.StatusUnauthorized || store.calls != 0 {
		t.Fatalf("anonymous author products request reached store: %d %d", w.Code, store.calls)
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
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "private, no-store" ||
		!strings.Contains(w.Body.String(), "Published learning product") ||
		store.actor != "00000000-0000-0000-0000-00000000b001" || store.calls != 1 {
		t.Fatalf("author product discovery did not bind to signed identity: %d %s", w.Code, w.Body.String())
	}
}
