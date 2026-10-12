package ucr

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/liveevents"
)

func TestTransportReadinessRejectsUnconfiguredAndUnhealthyUCR(t *testing.T) {
	var nilClient *Client
	if !errors.Is(nilClient.TransportReady(context.Background()), liveevents.ErrUnavailable) {
		t.Fatal("unconfigured provider advertised readiness")
	}
	now := time.Now
	for name, status := range map[string]int{
		"healthy": http.StatusOK,
		"unavailable": http.StatusServiceUnavailable,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/healthz" {
					t.Errorf("untrusted health path %q", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.WriteHeader(status)
				_, _ = w.Write([]byte("ok"))
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "machine-test-credential", "integration", "provider",
				"https://join.example", server.Client(), now)
			if err != nil {
				t.Fatal(err)
			}
			ready := client.TransportReady(context.Background())
			if (status == http.StatusOK) != (ready == nil) {
				t.Fatalf("status=%d readiness=%v", status, ready)
			}
		})
	}
}
