package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net"
	"net/url"
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

	httpClient, err := gatewayHTTPClient(os.Getenv("APGIC_EVENT_GATEWAY_URL"), os.Getenv("APGIC_EVENT_GATEWAY_DIAL_ADDR"))
	if err != nil {
		log.Fatalf("APGIC gateway dial configuration failed: %v", err)
	}
	deliverer, err := eventspine.NewHTTPDeliverer(
		os.Getenv("APGIC_EVENT_GATEWAY_URL"),
		os.Getenv("APGIC_EVENT_GATEWAY_PRINCIPAL_ID"),
		os.Getenv("APGIC_EVENT_GATEWAY_CREDENTIAL"),
		httpClient,
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

// gatewayHTTPClient routes a named HTTPS gateway to a loopback listener without
// changing the URL authority, HTTP Host header, TLS SNI or certificate validation.
// This avoids host-wide /etc/hosts overrides on a dedicated staging VPS.
func gatewayHTTPClient(endpoint, localDial string) (*http.Client, error) {
	if localDial == "" {
		return &http.Client{Timeout: 10 * time.Second}, nil
	}
	host, port, err := net.SplitHostPort(localDial)
	if err != nil || port != "443" || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return nil, fmt.Errorf("APGIC_EVENT_GATEWAY_DIAL_ADDR must be a loopback IP on port 443")
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Port() != "" {
		return nil, fmt.Errorf("loopback gateway requires a named HTTPS URL without a custom port")
	}
	targetHost := net.JoinHostPort(u.Hostname(), "443")
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if network != "tcp" || !strings.EqualFold(addr, targetHost) {
			return nil, fmt.Errorf("blocked unexpected gateway dial: %s", addr)
		}
		var dialer net.Dialer
		return dialer.DialContext(ctx, "tcp", localDial)
	}
	return &http.Client{Timeout: 10 * time.Second, Transport: transport}, nil
}
