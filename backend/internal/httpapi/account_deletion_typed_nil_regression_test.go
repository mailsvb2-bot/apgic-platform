package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/privacy"
)

// Models (*runtimepostgres.Checker)(nil) assigned to a Go interface in
// conformance mode. An unavailable store must never revoke unrelated users.
type nilDeletionRuntime struct{}

func (*nilDeletionRuntime) SubmitAccountDeletionRequest(context.Context, string, string) (privacy.AccountDeletionReceipt, error) {
	panic("typed nil deletion store was used")
}
func (*nilDeletionRuntime) ReadAccountDeletionRequest(context.Context, string) (privacy.AccountDeletionReceipt, error) {
	panic("typed nil deletion store was used")
}
func (*nilDeletionRuntime) ClientIdentityDeleted(context.Context, string) (bool, error) {
	panic("typed nil deletion store was used")
}

func TestConformanceSessionNeverBindsTypedNilDeletionStore(t *testing.T) {
	key := []byte(strings.Repeat("q", 48))
	var missing *nilDeletionRuntime
	handler := New(Options{
		Demand: demand.NewConformanceService(nil),
		ClientSessionKey: key,
		AccountDeletionIntake: missing,
	})
	create := func(cookie *http.Cookie) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/v1/help-intents",
			strings.NewReader(`{"free_text":"Сложности со сном"}`))
		request.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			request.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	first := create(nil)
	if first.Code != http.StatusCreated {
		t.Fatalf("conformance signed-session bootstrap failed: %d %s", first.Code, first.Body.String())
	}
	var cookie *http.Cookie
	for _, item := range first.Result().Cookies() {
		if item.Name == clientSessionCookieName {
			cookie = item
			break
		}
	}
	if cookie == nil {
		t.Fatal("conformance bootstrap did not issue canonical signed session")
	}
	second := create(cookie)
	if second.Code != http.StatusCreated {
		t.Fatalf("typed nil deletion store revoked a valid signed conformance session: %d %s", second.Code, second.Body.String())
	}
}
