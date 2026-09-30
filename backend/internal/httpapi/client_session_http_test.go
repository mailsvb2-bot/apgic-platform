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
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 3 || len(parts[2]) == 0 {
		t.Fatal("unexpected signed session token shape")
	}
	replacement := byte('A')
	if parts[2][0] == replacement {
		replacement = 'B'
	}
	parts[2] = string(replacement) + parts[2][1:]
	cookie.Value = strings.Join(parts, ".")

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

func TestHelpIntentOperationsRequireOwningTrustedSession(t *testing.T) {
	key := []byte(strings.Repeat("s", 32))
	service := demand.NewConformanceService(nil)
	intent, err := service.CreateIntentForIdentity(
		"11111111-1111-4111-8111-111111111111",
		"нужна помощь со сном",
	)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(Options{Demand: service, ClientSessionKey: key})
	manager, err := newClientSessionManager(key, nil)
	if err != nil {
		t.Fatal(err)
	}
	otherCookie, err := manager.issue("22222222-2222-4222-8222-222222222222")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{
			name:   "confirm",
			method: http.MethodPost,
			path:   "/v1/help-intents/" + intent.ID + "/confirm",
			body:   `{"topics":["sleep"]}`,
		},
		{
			name:   "matches",
			method: http.MethodGet,
			path:   "/v1/help-intents/" + intent.ID + "/matches",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+" cross identity", func(t *testing.T) {
			request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.body != "" {
				request.Header.Set("content-type", "application/json")
			}
			request.AddCookie(otherCookie)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), "HELP_INTENT_IDENTITY_MISMATCH") {
				t.Fatalf("ownership denial missing: %s", recorder.Body.String())
			}
		})

		t.Run(tt.name+" missing session", func(t *testing.T) {
			request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.body != "" {
				request.Header.Set("content-type", "application/json")
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), "CLIENT_SESSION_REQUIRED") {
				t.Fatalf("missing session denial missing: %s", recorder.Body.String())
			}
		})
	}
}

func TestProtectedClientIdentityInputsCannotOverrideTrustedSession(t *testing.T) {
	key := []byte(strings.Repeat("s", 32))
	handler := New(Options{
		Demand:           demand.NewConformanceService(nil),
		ClientSessionKey: key,
	})
	manager, err := newClientSessionManager(key, nil)
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := manager.issue("11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{
			name:   "slot hold",
			method: http.MethodPost,
			path:   "/v1/slot-holds",
			body:   `{"help_intent_id":"intent-x","slot_id":"slot-x","client_identity_id":"22222222-2222-4222-8222-222222222222"}`,
		},
		{
			name:   "checkout options",
			method: http.MethodGet,
			path:   "/v1/slot-holds/hold-x/checkout-options?client_identity_id=22222222-2222-4222-8222-222222222222",
		},
		{
			name:   "checkout instruction",
			method: http.MethodPost,
			path:   "/v1/checkout-instructions",
			body:   `{"hold_id":"hold-x","client_identity_id":"22222222-2222-4222-8222-222222222222","method_code":"BANK_CARD"}`,
		},
		{
			name:   "fulfillment",
			method: http.MethodGet,
			path:   "/v1/bookings/booking-x/fulfillment?identity_id=22222222-2222-4222-8222-222222222222",
		},
		{
			name:   "account deletion",
			method: http.MethodPost,
			path:   "/v1/account-deletions",
			body:   `{"identity_id":"22222222-2222-4222-8222-222222222222","source":"WEB"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.body != "" {
				request.Header.Set("content-type", "application/json")
			}
			request.AddCookie(cookie)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), "CLIENT_SESSION_IDENTITY_MISMATCH") {
				t.Fatalf("mismatch reason missing: %s", recorder.Body.String())
			}
		})
	}
}

func TestProtectedClientIdentityRequiresTrustedSession(t *testing.T) {
	handler := New(Options{
		Demand:           demand.NewConformanceService(nil),
		ClientSessionKey: []byte(strings.Repeat("s", 32)),
	})
	request := httptest.NewRequest(
		http.MethodGet,
		"/v1/slot-holds/hold-x/checkout-options?client_identity_id=11111111-1111-4111-8111-111111111111",
		nil,
	)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("missing session status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "CLIENT_SESSION_REQUIRED") {
		t.Fatalf("missing session reason missing: %s", recorder.Body.String())
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

func TestGrowthExportRequiresOwningTrustedSession(t *testing.T) {
	key := []byte(strings.Repeat("s", 32))
	ownerID := "11111111-1111-4111-8111-111111111111"
	otherID := "22222222-2222-4222-8222-222222222222"

	service := demand.NewConformanceService(nil)
	intent, err := service.CreateIntentForIdentity(ownerID, "нужна помощь со сном")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmIntent(intent.ID, []string{"sleep"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	slots, err := service.Slots("spec-lebedeva")
	if err != nil || len(slots) == 0 {
		t.Fatalf("slots=%#v err=%v", slots, err)
	}
	hold, err := service.AcquireHold(intent.ID, slots[0].ID, ownerID)
	if err != nil {
		t.Fatal(err)
	}

	handler := New(Options{Demand: service, ClientSessionKey: key})
	manager, err := newClientSessionManager(key, nil)
	if err != nil {
		t.Fatal(err)
	}
	otherCookie, err := manager.issue(otherID)
	if err != nil {
		t.Fatal(err)
	}

	cross := httptest.NewRequest(
		http.MethodPost,
		"/v1/consultations/"+hold.BookingID+"/growth-export",
		strings.NewReader(`{"purpose_consent":true}`),
	)
	cross.Header.Set("content-type", "application/json")
	cross.AddCookie(otherCookie)
	crossRecorder := httptest.NewRecorder()
	handler.ServeHTTP(crossRecorder, cross)
	if crossRecorder.Code != http.StatusForbidden {
		t.Fatalf("cross-subject growth export status=%d body=%s", crossRecorder.Code, crossRecorder.Body.String())
	}
	if !strings.Contains(crossRecorder.Body.String(), "DATA_SUBJECT_IDENTITY_MISMATCH") {
		t.Fatalf("cross-subject denial reason missing: %s", crossRecorder.Body.String())
	}

	missing := httptest.NewRequest(
		http.MethodPost,
		"/v1/consultations/"+hold.BookingID+"/growth-export",
		strings.NewReader(`{"purpose_consent":true}`),
	)
	missing.Header.Set("content-type", "application/json")
	missingRecorder := httptest.NewRecorder()
	handler.ServeHTTP(missingRecorder, missing)
	if missingRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("missing-session growth export status=%d body=%s", missingRecorder.Code, missingRecorder.Body.String())
	}
	if !strings.Contains(missingRecorder.Body.String(), "CLIENT_SESSION_REQUIRED") {
		t.Fatalf("missing-session denial reason missing: %s", missingRecorder.Body.String())
	}
}
