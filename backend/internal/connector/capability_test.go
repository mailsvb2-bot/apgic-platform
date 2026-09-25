package connector

import "testing"

func TestConnectorRequestRequiresIdempotencyAndSubject(t *testing.T) {
	for _, request := range []Request{
		{SubjectID: "booking-1"},
		{IdempotencyKey: "idem-1"},
	} {
		if err := request.Validate(); err == nil {
			t.Fatal("invalid connector request accepted")
		}
	}

	if err := (Request{
		IdempotencyKey: "idem-1",
		SubjectID:      "booking-1",
		Payload:        []byte(`{"ok":true}`),
	}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestConnectorRequestRejectsInvalidJSONPayload(t *testing.T) {
	request := Request{
		IdempotencyKey: "idem-1",
		SubjectID:      "booking-1",
		Payload:        []byte("{"),
	}
	if err := request.Validate(); err != ErrInvalidRequest {
		t.Fatalf("invalid JSON must fail, got %v", err)
	}
}

func TestProviderResultRequiresTypedOutcomeAndValidEvidence(t *testing.T) {
	if err := (Result{Outcome: Outcome("UNKNOWN")}).Validate(); err == nil {
		t.Fatal("unknown provider outcome must fail")
	}
	if err := (Result{Outcome: OutcomeSuccess, RawEvidence: []byte("{")}).Validate(); err == nil {
		t.Fatal("invalid provider evidence JSON must fail")
	}
}
