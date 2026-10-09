package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/launchconfig"
)

func readyConfig() launchconfig.Config {
	return launchconfig.Config{
		JurisdictionMatrixVersion: "jurisdiction-ci-v1",
		RetentionPolicyVersion:    "retention-ci-v1",
		SLOPolicyVersion:          "slo-ci-v1",
		ProviderMatrixVersion:     "providers-ci-v1",
	}
}

func TestPublicProviderEventCannotSelfAttestPayment(t *testing.T) {
	handler := New(Options{})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/provider-events", strings.NewReader(`{"outcome":"CAPTURED","provider_id":"external-bank","provider_event_id":"forged","order_id":"forged","amount_minor":100,"currency":"RUB"}`)))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("browser payment attestation status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if !contains(recorder.Body.String(), "PROVIDER_EVIDENCE_UNVERIFIED") {
		t.Fatalf("expected fail-closed provider boundary: %s", recorder.Body.String())
	}
	meta := httptest.NewRecorder()
	handler.ServeHTTP(meta, httptest.NewRequest(http.MethodGet, "/v1/meta", nil))
	if contains(meta.Body.String(), "\"conformance_provider_events\":true") {
		t.Fatalf("default runtime must not advertise synthetic captures: %s", meta.Body.String())
	}
}

func TestUnconfiguredExternalCheckoutFailsClosedAcrossWebAndNative(t *testing.T) {
	key := []byte(strings.Repeat("p", 32))
	service := demand.NewConformanceService(nil)
	handler := New(Options{
		Demand:           service,
		ClientMutations:  newMemoryMutationStore(),
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

	optionsRequest := httptest.NewRequest(http.MethodGet, "/v1/slot-holds/hold-x/checkout-options", nil)
	optionsRequest.AddCookie(cookie)
	optionsRecorder := httptest.NewRecorder()
	handler.ServeHTTP(optionsRecorder, optionsRequest)
	if optionsRecorder.Code != http.StatusOK {
		t.Fatalf("checkout options status=%d body=%s", optionsRecorder.Code, optionsRecorder.Body.String())
	}
	optionsBody := optionsRecorder.Body.String()
	for _, forbidden := range []string{"external-bank", "BANK_CARD", "SBP"} {
		if strings.Contains(optionsBody, forbidden) {
			t.Fatalf("unconfigured checkout exposed %q: %s", forbidden, optionsBody)
		}
	}
	if !strings.Contains(optionsBody, "\"external_execution_available\":false") || !strings.Contains(optionsBody, "\"options\":[]") {
		t.Fatalf("checkout options did not fail closed honestly: %s", optionsBody)
	}

	webRequest := httptest.NewRequest(http.MethodPost, "/v1/checkout-instructions", strings.NewReader(`{"hold_id":"hold-x","method_code":"BANK_CARD"}`))
	webRequest.Header.Set("content-type", "application/json")
	webRequest.AddCookie(cookie)
	webRecorder := httptest.NewRecorder()
	handler.ServeHTTP(webRecorder, webRequest)
	if webRecorder.Code != http.StatusServiceUnavailable || !strings.Contains(webRecorder.Body.String(), "PAY_EXTERNAL_PROVIDER_UNAVAILABLE") {
		t.Fatalf("web checkout did not fail closed: status=%d body=%s", webRecorder.Code, webRecorder.Body.String())
	}

	nativeRequest := httptest.NewRequest(http.MethodPost, "/v1/mobile/checkout-instructions", strings.NewReader(`{"hold_id":"hold-x","method_code":"BANK_CARD"}`))
	nativeRequest.Header.Set("content-type", "application/json")
	nativeRequest.Header.Set("Idempotency-Key", "production-like-checkout")
	nativeRequest.Header.Set("X-Correlation-Id", "corr-production-like-checkout")
	nativeRequest.AddCookie(cookie)
	nativeRecorder := httptest.NewRecorder()
	handler.ServeHTTP(nativeRecorder, nativeRequest)
	if nativeRecorder.Code != http.StatusServiceUnavailable || !strings.Contains(nativeRecorder.Body.String(), "PAY_EXTERNAL_PROVIDER_UNAVAILABLE") {
		t.Fatalf("native checkout did not fail closed: status=%d body=%s", nativeRecorder.Code, nativeRecorder.Body.String())
	}
}

func TestHealthDoesNotPretendToBeReadiness(t *testing.T) {
	handler := New(Options{})

	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health status = %d", health.Code)
	}

	ready := httptest.NewRecorder()
	handler.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if ready.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness status = %d, want %d", ready.Code, http.StatusServiceUnavailable)
	}
}

func TestReadinessRequiresExplicitVersionedLaunchConfig(t *testing.T) {
	handler := New(Options{LaunchConfig: readyConfig()})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("readiness status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestReadinessFailsClosedWhenStorageIsUnavailable(t *testing.T) {
	handler := New(Options{
		LaunchConfig: readyConfig(),
		ReadinessCheck: func(context.Context) error {
			return errors.New("database unavailable")
		},
	})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if !contains(recorder.Body.String(), "STORAGE_UNAVAILABLE") {
		t.Fatalf("readiness reason missing: %s", recorder.Body.String())
	}
}

func TestMetaIsStableAndEvidenceBearing(t *testing.T) {
	now := time.Date(2026, 9, 23, 17, 0, 0, 0, time.UTC)
	handler := New(Options{
		CommitSHA:    "abc123",
		LaunchConfig: readyConfig(),
		Now:          func() time.Time { return now },
	})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/meta", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("meta status = %d", recorder.Code)
	}
	body := recorder.Body.String()
	for _, expected := range []string{"abc123", "2026-09-23T17:00:00Z", "IOS", "ANDROID"} {
		if !contains(body, expected) {
			t.Fatalf("meta response missing %q: %s", expected, body)
		}
	}
}

func contains(value, part string) bool {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return true
		}
	}
	return false
}
