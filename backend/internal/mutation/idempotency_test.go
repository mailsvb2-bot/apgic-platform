package mutation

import "testing"

func TestRetryWithSameDigestIsDuplicate(t *testing.T) {
	existing := Envelope{IdentityID: "identity-1", Operation: "BOOKING_CONFIRM", IdempotencyKey: "key-1", RequestDigest: "sha256:a"}
	outcome, err := ClassifyRetry(existing, existing)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeDuplicate {
		t.Fatalf("expected DUPLICATE, got %s", outcome)
	}
}

func TestRetryWithChangedPayloadIsConflict(t *testing.T) {
	existing := Envelope{IdentityID: "identity-1", Operation: "BOOKING_CONFIRM", IdempotencyKey: "key-1", RequestDigest: "sha256:a"}
	incoming := existing
	incoming.RequestDigest = "sha256:b"
	outcome, err := ClassifyRetry(existing, incoming)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeConflict {
		t.Fatalf("expected CONFLICT, got %s", outcome)
	}
}
