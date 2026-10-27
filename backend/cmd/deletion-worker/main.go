package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/privacy"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/runtimepostgres"
)

func required(name string) string {
	v := os.Getenv(name)
	if v == "" {
		log.Fatalf("%s is required; deletion execution refused", name)
	}
	return v
}

func approvedPolicy(now time.Time) (privacy.VerifiedDeletionPolicy, error) {
	raw, err := os.ReadFile(required("APGIC_DELETION_APPROVED_POLICY_PATH"))
	if err != nil {
		return privacy.VerifiedDeletionPolicy{}, err
	}
	sigEncoded, err := os.ReadFile(required("APGIC_DELETION_APPROVED_SIGNATURE_PATH"))
	if err != nil {
		return privacy.VerifiedDeletionPolicy{}, err
	}
	key, err := base64.StdEncoding.DecodeString(required("APGIC_DELETION_APPROVAL_PUBLIC_KEY_B64"))
	if err != nil {
		return privacy.VerifiedDeletionPolicy{}, err
	}
	signature, err := base64.StdEncoding.DecodeString(string(sigEncoded))
	if err != nil {
		return privacy.VerifiedDeletionPolicy{}, err
	}
	return privacy.VerifyDeletionPolicy(raw, signature, ed25519.PublicKey(key), now)
}

func sweep(ctx context.Context, store *runtimepostgres.Checker, approved privacy.VerifiedDeletionPolicy) error {
	ids, err := store.PendingApprovedDeletionRequestIDs(ctx, 50)
	if err != nil {
		return err
	}
	for _, id := range ids {
		result, e := store.ExecuteApprovedAccountDeletionRequest(ctx, id, approved)
		switch {
		case e == nil:
			log.Printf("deletion request %s: canonical state %s (no erasure success implied before terminal evidence)", result.ID, result.State)
		case errors.Is(e, runtimepostgres.ErrDeletionBlockedByActiveObligations):
			log.Printf("deletion request %s blocked by active obligations", id)
		case errors.Is(e, runtimepostgres.ErrNoEligibleDeletionRequest):
			// Another worker won the race and has already handled the request.
		default:
			return e
		}
	}
	return nil
}

func main() {
	if os.Getenv("APGIC_DELETION_EXECUTE") != "1" {
		log.Fatal("APGIC_DELETION_EXECUTE=1 required; no implicit or CI-only erasure")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	store, err := runtimepostgres.Open(ctx, required("APGIC_DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	for {
		// Reverify signature and expiry for each iteration to fail closed
		// when a policy expires or is replaced on disk.
		approved, e := approvedPolicy(time.Now().UTC())
		if e != nil {
			log.Fatalf("deletion policy rejected: %v", e)
		}
		if e = sweep(ctx, store, approved); e != nil {
			log.Printf("deletion sweep failed, will retry: %v", e)
		}
		if os.Getenv("APGIC_DELETION_WORKER_ONCE") == "1" {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
		}
	}
}
