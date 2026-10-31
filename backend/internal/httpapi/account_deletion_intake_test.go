package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/privacy"
)

type deletionIntakeFixture struct {
	owner   string
	submits int
}

func (s *deletionIntakeFixture) SubmitAccountDeletionRequest(_ context.Context, owner, source string) (privacy.AccountDeletionReceipt, error) {
	s.owner = owner
	s.submits++
	return privacy.AccountDeletionReceipt{ID: "11111111-1111-4111-8111-111111111111", Source: source, State: "REQUESTED", CreatedAt: time.Now(), UpdatedAt: time.Now(), Notice: "Стирание не выполнено"}, nil
}
func (s *deletionIntakeFixture) ReadAccountDeletionRequest(_ context.Context, owner string) (privacy.AccountDeletionReceipt, error) {
	s.owner = owner
	return privacy.AccountDeletionReceipt{ID: "11111111-1111-4111-8111-111111111111", State: "REQUESTED"}, nil
}

func TestDeletionIntakeRequiresSignedOwnerAndNeverClaimsErasure(t *testing.T) {
	key := []byte(strings.Repeat("z", 48))
	st := &deletionIntakeFixture{}
	handler := New(Options{ClientSessionKey: key, AccountDeletionIntake: st})
	sessions, err := newClientSessionManager(key, nil)
	if err != nil {
		t.Fatal(err)
	}
	const owner = "22222222-2222-4222-8222-222222222222"
	cookie, err := sessions.issue(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{"source":"WEB","identity_id":"forged"}`, `{"source":"UNKNOWN"}`, `{"source":"WEB"}{}`} {
		r := httptest.NewRequest(http.MethodPost, "/v1/client/account-deletion-requests", strings.NewReader(body))
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusBadRequest || st.submits != 0 {
			t.Fatalf("invalid deletion body reached store: %d %s", w.Code, w.Body.String())
		}
	}
	anonymous := httptest.NewRequest(http.MethodPost, "/v1/client/account-deletion-requests", strings.NewReader(`{"source":"WEB"}`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, anonymous)
	if w.Code != http.StatusUnauthorized || st.submits != 0 {
		t.Fatalf("unsigned request reached store: %d", w.Code)
	}
	signed := httptest.NewRequest(http.MethodPost, "/v1/client/account-deletion-requests", strings.NewReader(`{"source":"WEB"}`))
	signed.AddCookie(cookie)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, signed)
	if w.Code != http.StatusAccepted || st.owner != owner || st.submits != 1 {
		t.Fatalf("signed request failed: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "profile_erased") || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
		t.Fatal("deletion intake exposed false erasure or cacheable data")
	}
}
