package runtimepostgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/mutation"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

func TestClientMutationPersistsAppliedFailedAndConflictOutcomes(t *testing.T) {
	databaseURL := os.Getenv("APGIC_CLIENT_MUTATION_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("client mutation integration database not configured")
	}
	store, err := Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	identityID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO identities (id) VALUES ($1::uuid)`, identityID); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	mutationID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	envelope := mutation.Envelope{
		IdentityID:     identityID,
		Operation:      "CREATE_CHECKOUT",
		IdempotencyKey: "runtime-checkout-1",
		RequestDigest:  "sha256:payload-a",
	}

	first, err := store.Claim(context.Background(), mutationID, envelope, now)
	if err != nil {
		t.Fatal(err)
	}
	if first.Outcome != mutation.OutcomeClaimed || first.State != mutation.StateClaimed || first.MutationID != mutationID {
		t.Fatalf("first claim = %#v", first)
	}

	retryID, _ := persistentid.New()
	retry, err := store.Claim(context.Background(), retryID, envelope, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if retry.Outcome != mutation.OutcomeDuplicate || retry.MutationID != mutationID {
		t.Fatalf("retry claim = %#v", retry)
	}

	changed := envelope
	changed.RequestDigest = "sha256:payload-b"
	conflictID, _ := persistentid.New()
	conflict, err := store.Claim(context.Background(), conflictID, changed, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if conflict.Outcome != mutation.OutcomeConflict || conflict.MutationID != mutationID {
		t.Fatalf("changed payload = %#v", conflict)
	}

	failed, err := store.Fail(context.Background(), mutationID, "PAY_METHOD_UNSUPPORTED", now.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !failed.Changed || failed.ReasonCode != "MUTATION_FAILED" {
		t.Fatalf("fail result = %#v", failed)
	}
	failedReplayID, _ := persistentid.New()
	failedReplay, err := store.Claim(context.Background(), failedReplayID, envelope, now.Add(4*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if failedReplay.Outcome != mutation.OutcomeFailed || failedReplay.State != mutation.StateFailed ||
		failedReplay.FailureCode != "PAY_METHOD_UNSUPPORTED" {
		t.Fatalf("failed replay = %#v", failedReplay)
	}
	finalizeFailed, err := store.Finalize(context.Background(), mutationID, "checkout/should-not-apply", now.Add(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if finalizeFailed.Changed || finalizeFailed.ReasonCode != "MUTATION_ALREADY_FAILED" {
		t.Fatalf("finalize failed mutation = %#v", finalizeFailed)
	}

	appliedID, _ := persistentid.New()
	appliedEnvelope := mutation.Envelope{
		IdentityID:     identityID,
		Operation:      "CREATE_CHECKOUT",
		IdempotencyKey: "runtime-checkout-2",
		RequestDigest:  "sha256:payload-c",
	}
	appliedClaim, err := store.Claim(context.Background(), appliedID, appliedEnvelope, now.Add(6*time.Second))
	if err != nil || appliedClaim.Outcome != mutation.OutcomeClaimed {
		t.Fatalf("applied claim = %#v err=%v", appliedClaim, err)
	}
	finalized, err := store.Finalize(context.Background(), appliedID, "checkout/checkout-1", now.Add(7*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !finalized.Changed || finalized.ReasonCode != "MUTATION_APPLIED" {
		t.Fatalf("finalize result = %#v", finalized)
	}
	appliedReplayID, _ := persistentid.New()
	appliedReplay, err := store.Claim(context.Background(), appliedReplayID, appliedEnvelope, now.Add(8*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if appliedReplay.Outcome != mutation.OutcomeDuplicateApplied ||
		appliedReplay.SideEffectRef != "checkout/checkout-1" ||
		appliedReplay.State != mutation.StateApplied {
		t.Fatalf("applied replay = %#v", appliedReplay)
	}
}
