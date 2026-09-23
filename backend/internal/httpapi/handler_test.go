package httpapi

import (
	"net/http"
	"net/http/httptest"
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
