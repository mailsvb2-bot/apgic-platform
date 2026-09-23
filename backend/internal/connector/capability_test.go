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
	}).Validate(); err != nil {
		t.Fatal(err)
	}
}
