// finance-worker runs the R4 non-custodial financial orchestration loop:
// it turns COMPLETED + captured consultations into provider-executed
// settlement instructions (delivered by the outbox worker through the
// connector gateway) and periodically reconciles provider statements against
// ledger-backed facts. It never executes, holds or moves money itself.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/runtimepostgres"
)

const (
	defaultBatchSize         = 100
	defaultPollInterval      = 5 * time.Second
	defaultReconcileInterval = time.Hour
	reconciliationLookback   = 48 * time.Hour
)

type settings struct {
	batchSize         int
	pollInterval      time.Duration
	reconcileInterval time.Duration
}

func main() {
	cfg, err := workerSettings()
	if err != nil {
		log.Fatalf("APGIC finance worker configuration failed: %v", err)
	}
	storage, err := runtimepostgres.Open(context.Background(), os.Getenv("APGIC_DATABASE_URL"))
	if err != nil {
		log.Fatalf("APGIC finance PostgreSQL readiness failed: %v", err)
	}
	defer storage.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	log.Printf("APGIC finance worker started batch=%d poll=%s reconcile=%s", cfg.batchSize, cfg.pollInterval, cfg.reconcileInterval)

	ticker := time.NewTicker(cfg.pollInterval)
	defer ticker.Stop()
	var lastReconcile time.Time
	for {
		outcomes, err := storage.SettleCompletedConsultations(ctx, cfg.batchSize)
		if err != nil && ctx.Err() == nil {
			log.Printf("APGIC finance settlement cycle incomplete: processed=%d error=%v", len(outcomes), err)
		}
		for _, outcome := range outcomes {
			log.Printf("APGIC finance settlement order=%s status=%s reason=%s", outcome.OrderID, outcome.Status, outcome.ReasonCode)
		}
		if now := time.Now().UTC(); now.Sub(lastReconcile) >= cfg.reconcileInterval {
			reconcileAll(ctx, storage, now, cfg.reconcileInterval)
			lastReconcile = now
		}
		select {
		case <-ctx.Done():
			log.Printf("APGIC finance worker stopped")
			return
		case <-ticker.C:
		}
	}
}

func reconcileAll(ctx context.Context, storage *runtimepostgres.Checker, now time.Time, bucket time.Duration) {
	start, end := reconciliationWindow(now, bucket)
	providers, err := storage.FinanceProviderInstances(ctx)
	if err != nil {
		log.Printf("APGIC finance reconciliation scope unavailable: %v", err)
		return
	}
	for _, provider := range providers {
		report, err := storage.ReconcileFinanceProvider(ctx, provider, start, end)
		if err != nil {
			log.Printf("APGIC finance reconciliation provider=%s failed: %v", provider, err)
			continue
		}
		log.Printf("APGIC finance reconciliation provider=%s run=%s matched=%d mismatched=%d pending=%d",
			provider, report.RunID, report.Matched, report.Mismatched, report.Pending)
	}
}

// reconciliationWindow aligns runs to bucket boundaries so a restarted worker
// replays the same run id instead of creating overlapping history.
func reconciliationWindow(now time.Time, bucket time.Duration) (time.Time, time.Time) {
	end := now.Truncate(bucket)
	return end.Add(-reconciliationLookback), end
}

func workerSettings() (settings, error) {
	cfg := settings{batchSize: defaultBatchSize, pollInterval: defaultPollInterval, reconcileInterval: defaultReconcileInterval}
	if raw := strings.TrimSpace(os.Getenv("APGIC_FINANCE_BATCH_SIZE")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 || value > 1000 {
			return cfg, fmt.Errorf("APGIC_FINANCE_BATCH_SIZE must be within 1..1000")
		}
		cfg.batchSize = value
	}
	if raw := strings.TrimSpace(os.Getenv("APGIC_FINANCE_POLL_INTERVAL_MS")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 500 || value > 300000 {
			return cfg, fmt.Errorf("APGIC_FINANCE_POLL_INTERVAL_MS must be within 500..300000")
		}
		cfg.pollInterval = time.Duration(value) * time.Millisecond
	}
	if raw := strings.TrimSpace(os.Getenv("APGIC_FINANCE_RECONCILE_INTERVAL_MINUTES")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 5 || value > 1440 {
			return cfg, fmt.Errorf("APGIC_FINANCE_RECONCILE_INTERVAL_MINUTES must be within 5..1440")
		}
		cfg.reconcileInterval = time.Duration(value) * time.Minute
	}
	return cfg, nil
}
