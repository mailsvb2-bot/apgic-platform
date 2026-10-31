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

func TestFinanceReconciliationSchedulingNeverTreatsFailureAsSuccess(t *testing.T) {
	now := time.Date(2026, 10, 16, 9, 13, 0, 0, time.UTC)
	for _, tc := range []struct {
		name       string
		interval   time.Duration
		successful bool
		wantDelay  time.Duration
	}{
		{name: "provider unavailable", interval: time.Hour, wantDelay: time.Minute},
		{name: "provider statement missing", interval: 5 * time.Minute, wantDelay: time.Minute},
		{name: "complete reconciliation", interval: time.Hour, successful: true, wantDelay: time.Hour},
		{name: "short configured interval", interval: 30 * time.Second, wantDelay: 30 * time.Second},
		{name: "short successful interval", interval: 30 * time.Second, successful: true, wantDelay: 30 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := nextFinanceReconcileAttempt(now, tc.interval, tc.successful)
			if want := now.Add(tc.wantDelay); !next.Equal(want) {
				t.Fatalf("reconciliation retry=%s, want %s", next, want)
			}
			if next.Before(now) {
				t.Fatalf("reconciliation attempted in the past: %s", next)
			}
		})
	}
}

func TestFinanceReconciliationRetryUsesSameDurableBucket(t *testing.T) {
	at := time.Date(2026, 10, 16, 9, 13, 0, 0, time.UTC)
	start, end := reconciliationWindow(at, time.Hour)
	retry := nextFinanceReconcileAttempt(at, time.Hour, false)
	retryStart, retryEnd := reconciliationWindow(retry, time.Hour)
	if !start.Equal(retryStart) || !end.Equal(retryEnd) {
		t.Fatalf("retry must use same canonical reconciliation window: original=%s..%s retry=%s..%s", start, end, retryStart, retryEnd)
	}
}
