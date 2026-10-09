package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/eventspine"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/runtimepostgres"
)

const (
	defaultBatchSize    = 100
	defaultPollInterval = time.Second
)

func main() {
	batchSize, pollInterval, err := workerSettings()
	if err != nil {
		log.Fatalf("APGIC outbox worker configuration failed: %v", err)
	}
	storage, err := runtimepostgres.Open(context.Background(), os.Getenv("APGIC_DATABASE_URL"))
	if err != nil {
		log.Fatalf("APGIC outbox PostgreSQL readiness failed: %v", err)
	}
	defer storage.Close()

	deliverer, err := eventspine.NewHTTPDeliverer(
		os.Getenv("APGIC_EVENT_GATEWAY_URL"),
		os.Getenv("APGIC_EVENT_GATEWAY_PRINCIPAL_ID"),
		os.Getenv("APGIC_EVENT_GATEWAY_CREDENTIAL"),
		&http.Client{Timeout: 10 * time.Second},
	)
	if err != nil {
		log.Fatalf("APGIC outbox event gateway configuration failed: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	log.Printf("APGIC outbox worker started batch=%d poll=%s", batchSize, pollInterval)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		delivered, err := storage.DeliverPendingOutbox(ctx, batchSize, deliverer.Deliver)
		if err != nil && ctx.Err() == nil {
			log.Printf("APGIC outbox delivery cycle incomplete: delivered=%d error=%v", delivered, err)
		}
		select {
		case <-ctx.Done():
			log.Printf("APGIC outbox worker stopped")
			return
		case <-ticker.C:
		}
	}
}

func workerSettings() (int, time.Duration, error) {
	batchSize := defaultBatchSize
	if raw := strings.TrimSpace(os.Getenv("APGIC_OUTBOX_BATCH_SIZE")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 || value > 1000 {
			return 0, 0, fmt.Errorf("APGIC_OUTBOX_BATCH_SIZE must be within 1..1000")
		}
		batchSize = value
	}
	poll := defaultPollInterval
	if raw := strings.TrimSpace(os.Getenv("APGIC_OUTBOX_POLL_INTERVAL_MS")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 100 || value > 60000 {
			return 0, 0, fmt.Errorf("APGIC_OUTBOX_POLL_INTERVAL_MS must be within 100..60000")
		}
		poll = time.Duration(value) * time.Millisecond
	}
	return batchSize, poll, nil
}
