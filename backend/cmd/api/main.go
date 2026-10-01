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
	var ledgerStore demand.LedgerStore
	var journeyStore demand.JourneyStore
	environment := os.Getenv("APGIC_ENVIRONMENT")
	clientSessionKey := []byte(os.Getenv("APGIC_CLIENT_SESSION_KEY"))
	if runtimepostgres.RequiresDatabase(environment) {
		if err := httpapi.ValidateClientSessionKey(clientSessionKey); err != nil {
			log.Fatalf("APGIC client session configuration failed: %v", err)
		}
		var err error
		storage, err = runtimepostgres.Open(context.Background(), os.Getenv("APGIC_DATABASE_URL"))
		if err != nil {
			log.Fatalf("APGIC PostgreSQL readiness failed: %v", err)
		}
		defer storage.Close()
		readinessCheck = storage.Ready
		ledgerStore = storage
		journeyStore = storage
	}
	var demandService *demand.Service
	if journeyStore != nil {
		var err error
		demandService, err = demand.NewConformanceServiceWithStoresAndSpecialists(nil, ledgerStore, journeyStore, storage)
		if err != nil {
			log.Fatalf("APGIC journey hydration failed: %v", err)
		}
	} else {
		demandService = demand.NewConformanceServiceWithLedgerStore(nil, ledgerStore)
	}
	handler := httpapi.New(httpapi.Options{
		CommitSHA:        os.Getenv("APGIC_COMMIT_SHA"),
		ReleaseTrack:     "R0",
		Demand:           demandService,
		ClientSessionKey: clientSessionKey,
		ReadinessCheck:   readinessCheck,
		LegalAcceptances: storage,
		Installations:    storage,
		Specialists:      storage,
		OrganizationAuth: storage,
		Organizations:    storage,
		ProductOwnership: storage,
		Products:         storage,
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
