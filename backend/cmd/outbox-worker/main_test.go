package main

import (
	"os"
	"testing"
	"time"
)

func TestWorkerSettingsDefaultsAndBounds(t *testing.T) {
	for _, key := range []string{"APGIC_OUTBOX_BATCH_SIZE", "APGIC_OUTBOX_POLL_INTERVAL_MS"} {
		old, ok := os.LookupEnv(key)
		_ = os.Unsetenv(key)
		defer func(key, old string, ok bool) {
			if ok {
				_ = os.Setenv(key, old)
			} else {
				_ = os.Unsetenv(key)
			}
		}(key, old, ok)
	}
	batch, poll, err := workerSettings()
	if err != nil || batch != 100 || poll != time.Second {
		t.Fatalf("defaults batch=%d poll=%s err=%v", batch, poll, err)
	}
	_ = os.Setenv("APGIC_OUTBOX_BATCH_SIZE", "1001")
	if _, _, err := workerSettings(); err == nil {
		t.Fatal("oversized batch must fail")
	}
	_ = os.Setenv("APGIC_OUTBOX_BATCH_SIZE", "100")
	_ = os.Setenv("APGIC_OUTBOX_POLL_INTERVAL_MS", "99")
	if _, _, err := workerSettings(); err == nil {
		t.Fatal("too-fast poll interval must fail")
	}
}

func TestGatewayHTTPClientLoopbackSafety(t *testing.T) {
	for _, address := range []string{"127.0.0.1:43114", "0.0.0.0:443", "192.0.2.10:443", "example.com:443", "[::]:443", "garbage"} {
		if _, err := gatewayHTTPClient("https://apgic.ru/_internal/apgic-event-gateway", address); err == nil {
			t.Errorf("unsafe gateway dial address accepted: %s", address)
		}
	}
	for _, endpoint := range []string{"http://apgic.ru/", "https://user:pass@apgic.ru/", "https://apgic.ru:8443/"} {
		if _, err := gatewayHTTPClient(endpoint, "127.0.0.1:443"); err == nil {
			t.Errorf("unsafe gateway endpoint accepted: %s", endpoint)
		}
	}
	c, err := gatewayHTTPClient("https://apgic.ru/_internal/apgic-event-gateway", "127.0.0.1:443")
	if err != nil || c == nil || c.Transport == nil {
		t.Fatalf("loopback TLS client: %v", err)
	}
	if _, err := gatewayHTTPClient("https://apgic.ru/", "[::1]:443"); err != nil {
		t.Fatalf("IPv6 loopback rejected: %v", err)
	}
	if _, err := gatewayHTTPClient("https://apgic.ru/", ""); err != nil {
		t.Fatalf("default client rejected: %v", err)
	}
}
