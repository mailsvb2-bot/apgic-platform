package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/httpapi"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/launchconfig"
)

func main() {
	handler := httpapi.New(httpapi.Options{
		CommitSHA:    os.Getenv("APGIC_COMMIT_SHA"),
		ReleaseTrack: "R0",
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
