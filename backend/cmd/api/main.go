package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/httpapi"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/launchconfig"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/runtimepostgres"
)

func main() {
	var readinessCheck func(context.Context) error
	var storage *runtimepostgres.Checker
	environment := os.Getenv("APGIC_ENVIRONMENT")
	if runtimepostgres.RequiresDatabase(environment) {
		var err error
		storage, err = runtimepostgres.Open(context.Background(), os.Getenv("APGIC_DATABASE_URL"))
		if err != nil {
			log.Fatalf("APGIC PostgreSQL readiness failed: %v", err)
		}
		defer storage.Close()
		readinessCheck = storage.Ready
	}
	handler := httpapi.New(httpapi.Options{
		CommitSHA:      os.Getenv("APGIC_COMMIT_SHA"),
		ReleaseTrack:   "R0",
		Demand:         demand.NewConformanceService(nil),
		ReadinessCheck: readinessCheck,
		LaunchConfig: launchconfig.Config{
			JurisdictionMatrixVersion: os.Getenv("APGIC_JURISDICTION_MATRIX_VERSION"),
			RetentionPolicyVersion:    os.Getenv("APGIC_RETENTION_POLICY_VERSION"),
			SLOPolicyVersion:          os.Getenv("APGIC_SLO_POLICY_VERSION"),
			ProviderMatrixVersion:     os.Getenv("APGIC_PROVIDER_MATRIX_VERSION"),
		},
	})

	addr := envOr("APGIC_HTTP_ADDR", ":8080")
	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("APGIC API listening on %s", addr)
	log.Fatal(server.ListenAndServe())
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
