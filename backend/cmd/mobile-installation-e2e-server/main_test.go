package main

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHelpIntentOutcomeDiagnosticDoesNotExposeSession(t *testing.T) {
	var out bytes.Buffer
	original := log.Writer()
	log.SetOutput(&out)
	defer log.SetOutput(original)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/help-intents/some-secret-identity/confirm" {
			t.Fatalf("unexpected route: %s", r.URL.Path)
		}
		w.Header().Set("X-Test-Forwarded", "true")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("safe response"))
	})
	req := httptest.NewRequest(http.MethodPost,
		"/v1/help-intents/some-secret-identity/confirm", nil)
	req.Header.Set("Cookie", "__Host-apgic_session=secret-cookie-material")
	rec := httptest.NewRecorder()
	withHelpIntentOutcomeEvidence(next).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized || rec.Body.String() != "safe response" ||
		rec.Header().Get("X-Test-Forwarded") != "true" {
		t.Fatalf("diagnostic changed canonical HTTP response: %#v", rec)
	}
	logged := out.String()
	if !strings.Contains(logged, "CONFIRM status=401 session_cookie_present=true") {
		t.Fatalf("missing outcome diagnostics: %s", logged)
	}
	for _, sensitive := range []string{"some-secret-identity", "secret-cookie-material", "__Host-apgic_session"} {
		if strings.Contains(logged, sensitive) {
			t.Fatalf("session or identity leaked to logs")
		}
	}
}
