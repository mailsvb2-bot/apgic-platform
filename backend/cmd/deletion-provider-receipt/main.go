package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"log"
	"os"
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/privacy"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/runtimepostgres"
	"time"
)

func required(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("%s is required", name)
	}
	return value
}

func main() {
	ctx := context.Background()
	raw, err := os.ReadFile(required("APGIC_PROVIDER_ERASURE_RECEIPT_PATH"))
	if err != nil {
		log.Fatal(err)
	}
	sigFile, err := os.ReadFile(required("APGIC_PROVIDER_ERASURE_SIGNATURE_PATH"))
	if err != nil {
		log.Fatal(err)
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sigFile)))
	if err != nil {
		log.Fatal(err)
	}
	key, err := base64.StdEncoding.DecodeString(required("APGIC_PROVIDER_ERASURE_PUBLIC_KEY_B64"))
	if err != nil {
		log.Fatal(err)
	}
	verified, err := privacy.VerifyProviderErasureReceipt(raw, sig, ed25519.PublicKey(key),
		required("APGIC_PROVIDER_ERASURE_REF"), time.Now().UTC())
	if err != nil {
		log.Fatalf("unverifiable deletion receipt: %v", err)
	}
	store, err := runtimepostgres.Open(ctx, required("APGIC_DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	result, err := store.MarkVerifiedProviderErasure(ctx, verified)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("verified external erasure request=%s state=%s idempotent=%t", result.ID, result.State, result.Idempotent)
}
