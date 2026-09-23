package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

type metaResponse struct {
	Service      string   `json:"service"`
	ReleaseTrack string   `json:"release_track"`
	Surfaces     []string `json:"surfaces"`
	CommitSHA    string   `json:"commit_sha,omitempty"`
	Time         string   `json:"time"`
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /v1/meta", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(metaResponse{
			Service:      "apgic-api",
			ReleaseTrack: "R0",
			Surfaces:     []string{"WEB", "PWA", "IOS", "ANDROID"},
			CommitSHA:    os.Getenv("APGIC_COMMIT_SHA"),
			Time:         time.Now().UTC().Format(time.RFC3339),
		})
	})

	addr := envOr("APGIC_HTTP_ADDR", ":8080")
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
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
