package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/internal/platform/httpserver"
)

func main() {
	addr := os.Getenv("APGIC_HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           httpserver.New(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("apgic-api listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
