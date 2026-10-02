package httpapi

import (
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/remoteconfig"
)

func TestMobileRemoteConfigReturnsVerifiableSignedEnvelope(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 2, 20, 30, 0, 0, time.UTC)
	publisher, err := remoteconfig.NewPublisher(
		"mobile027-test-key",
		privateKey,
		7,
		"mobile027-policy-v1",
		15*time.Minute,
		[]remoteconfig.Capability{remoteconfig.CapabilityRealtimeConsultation},
		map[remoteconfig.Capability]string{
			remoteconfig.CapabilityRealtimeConsultation: "INCIDENT_DISABLE_REALTIME",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(Options{
		RemoteConfigProvider: publisher.Envelope,
		Now:                  func() time.Time { return now },
	})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/mobile/remote-config", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var envelope remoteconfig.SignedEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Payload.Version != 7 || envelope.Payload.PolicyID != "mobile027-policy-v1" {
		t.Fatalf("unexpected payload: %+v", envelope.Payload)
	}
	if !envelope.Payload.ExpiresAt.Equal(now.Add(15 * time.Minute)) {
		t.Fatalf("unexpected expiry: %s", envelope.Payload.ExpiresAt)
	}
	if err := remoteconfig.Verify(envelope, publicKey, now); err != nil {
		t.Fatalf("server returned unverifiable envelope: %v", err)
	}
}

func TestMobileRemoteConfigFailsClosedWhenProviderUnavailable(t *testing.T) {
	handler := New(Options{})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/mobile/remote-config", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if body := recorder.Body.String(); body == "" || !containsAll(body, "REMOTE_CONFIG_UNAVAILABLE", "retryable") {
		t.Fatalf("unexpected body: %s", body)
	}
}

func containsAll(value string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(value, needle) {
			return false
		}
	}
	return true
}
