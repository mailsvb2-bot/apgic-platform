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
