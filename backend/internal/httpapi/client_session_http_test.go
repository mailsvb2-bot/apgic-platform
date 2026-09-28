package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
)

func TestHelpIntentReusesTrustedClientSessionIdentity(t *testing.T) {
	handler := New(Options{
		Demand:           demand.NewConformanceService(nil),
		ClientSessionKey: []byte(strings.Repeat("s", 32)),
	})

	create := func(cookie *http.Cookie, freeText string) (*demand.Intent, *http.Cookie, int) {
		request := httptest.NewRequest(
			http.MethodPost,
			"/v1/help-intents",
			strings.NewReader(`{"free_text":"`+freeText+`"}`),
		)
		request.Header.Set("content-type", "application/json")
		if cookie != nil {
			request.AddCookie(cookie)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		var intent demand.Intent
		if recorder.Code == http.StatusCreated {
			if err := json.Unmarshal(recorder.Body.Bytes(), &intent); err != nil {
				t.Fatal(err)
			}
		}
		var issued *http.Cookie
		for _, candidate := range recorder.Result().Cookies() {
			if candidate.Name == clientSessionCookieName {
				issued = candidate
				break
			}
		}
		return &intent, issued, recorder.Code
	}

	first, cookie, status := create(nil, "первый запрос")
	if status != http.StatusCreated || cookie == nil {
		t.Fatalf("first create status=%d cookie=%#v", status, cookie)
	}
	second, rotated, status := create(cookie, "второй запрос")
	if status != http.StatusCreated {
		t.Fatalf("second create status=%d", status)
	}
	if rotated != nil {
		t.Fatal("existing trusted session must not rotate identity cookie")
	}
	if first.ID == second.ID {
		t.Fatal("help intent ids must remain distinct")
	}
	if first.ClientIdentityID == "" || first.ClientIdentityID != second.ClientIdentityID {
		t.Fatalf("trusted session split identity: first=%q second=%q", first.ClientIdentityID, second.ClientIdentityID)
	}
}

func TestHelpIntentRejectsTamperedTrustedSession(t *testing.T) {
	handler := New(Options{
		Demand:           demand.NewConformanceService(nil),
		ClientSessionKey: []byte(strings.Repeat("s", 32)),
	})
	manager, err := newClientSessionManager([]byte(strings.Repeat("s", 32)), nil)
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := manager.issue("11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	cookie.Value = cookie.Value[:len(cookie.Value)-1] + "A"

	request := httptest.NewRequest(http.MethodPost, "/v1/help-intents", strings.NewReader(`{"free_text":"запрос"}`))
	request.Header.Set("content-type", "application/json")
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("tampered session status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "CLIENT_SESSION_INVALID") {
		t.Fatalf("tampered session reason missing: %s", recorder.Body.String())
	}
}

func TestHelpIntentFailsClosedOnWeakConfiguredSessionKey(t *testing.T) {
	handler := New(Options{
		Demand:           demand.NewConformanceService(nil),
		ClientSessionKey: []byte("weak"),
	})
	request := httptest.NewRequest(http.MethodPost, "/v1/help-intents", strings.NewReader(`{"free_text":"запрос"}`))
	request.Header.Set("content-type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("weak session key status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "CLIENT_SESSION_UNAVAILABLE") {
		t.Fatalf("weak session key reason missing: %s", recorder.Body.String())
	}
}
