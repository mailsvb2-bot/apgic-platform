package main

import (
	"testing"
	"time"
)

func TestFinanceWorkerSettingsDefaultsAndBounds(t *testing.T) {
	keys := []string{"APGIC_FINANCE_BATCH_SIZE", "APGIC_FINANCE_POLL_INTERVAL_MS", "APGIC_FINANCE_RECONCILE_INTERVAL_MINUTES"}
	for _, key := range keys {
		t.Setenv(key, "")
	}
	cfg, err := workerSettings()
	if err != nil || cfg.batchSize != 100 || cfg.pollInterval != 5*time.Second || cfg.reconcileInterval != time.Hour {
		t.Fatalf("defaults %+v err=%v", cfg, err)
	}
	for key, bad := range map[string]string{keys[0]: "1001", keys[1]: "100", keys[2]: "1"} {
		t.Setenv(key, bad)
		if _, err := workerSettings(); err == nil {
			t.Fatalf("%s=%s must fail", key, bad)
		}
		t.Setenv(key, "")
	}
}

func TestReconciliationWindowIsBucketAlignedAndReplayable(t *testing.T) {
	now := time.Date(2026, 10, 15, 3, 47, 12, 0, time.UTC)
	start, end := reconciliationWindow(now, time.Hour)
	again, againEnd := reconciliationWindow(now.Add(10*time.Minute), time.Hour)
	if !end.Equal(time.Date(2026, 10, 15, 3, 0, 0, 0, time.UTC)) || !start.Equal(end.Add(-48*time.Hour)) {
		t.Fatalf("window %s..%s", start, end)
	}
	if !start.Equal(again) || !end.Equal(againEnd) {
		t.Fatal("restart inside one bucket must replay the same run window")
	}
}
