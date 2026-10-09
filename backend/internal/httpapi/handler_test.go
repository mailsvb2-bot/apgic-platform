package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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


func TestPublicCheckoutDoesNotExposeConformancePaymentByDefault(t *testing.T) {
	handler := New(Options{})

	options := httptest.NewRecorder()
	handler.ServeHTTP(options, httptest.NewRequest(http.MethodGet, "/v1/slot-holds/forged/checkout-options", nil))
	if options.Code != http.StatusOK {
		t.Fatalf("checkout options status = %d, body=%s", options.Code, options.Body.String())
	}
	if !contains(options.Body.String(), "EXTERNAL_PAYMENT_UNAVAILABLE") {
		t.Fatalf("expected unavailable payment reason: %s", options.Body.String())
	}
	if contains(options.Body.String(), "external-bank") || contains(options.Body.String(), "BANK_CARD") || contains(options.Body.String(), "SBP") {
		t.Fatalf("public runtime exposed conformance payment methods: %s", options.Body.String())
	}

	checkout := httptest.NewRecorder()
	handler.ServeHTTP(checkout, httptest.NewRequest(http.MethodPost, "/v1/checkout-instructions", strings.NewReader(`{"hold_id":"forged","client_identity_id":"forged","method_code":"BANK_CARD"}`)))
	if checkout.Code != http.StatusServiceUnavailable {
		t.Fatalf("checkout status = %d, body=%s", checkout.Code, checkout.Body.String())
	}
	if !contains(checkout.Body.String(), "EXTERNAL_PAYMENT_UNAVAILABLE") {
		t.Fatalf("expected fail-closed checkout boundary: %s", checkout.Body.String())
	}

	mobile := httptest.NewRecorder()
	handler.ServeHTTP(mobile, httptest.NewRequest(http.MethodPost, "/v1/mobile/checkout-instructions", strings.NewReader(`{"hold_id":"forged","method_code":"BANK_CARD"}`)))
	if mobile.Code != http.StatusServiceUnavailable {
		t.Fatalf("mobile checkout status = %d, body=%s", mobile.Code, mobile.Body.String())
	}
	if !contains(mobile.Body.String(), "EXTERNAL_PAYMENT_UNAVAILABLE") {
		t.Fatalf("expected fail-closed mobile checkout boundary: %s", mobile.Body.String())
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
