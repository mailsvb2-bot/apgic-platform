package connectors

import "testing"

func TestConnectorRequestRequiresIdempotencyKey(t *testing.T) {
	request := Request{SubjectID: "booking-1"}
	if err := request.Validate(); err == nil {
		t.Fatal("connector requests must be idempotent by contract")
	}
}
