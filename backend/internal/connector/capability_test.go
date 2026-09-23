package connector

import "testing"

func TestConnectorRequestRequiresIdempotencyAndSubject(t *testing.T) {
	for _,r:=range []Request{{SubjectID:"booking-1"},{IdempotencyKey:"idem-1"}} {
		if err:=r.Validate(); err==nil { t.Fatal("invalid connector request accepted") }
	}
	if err:=(Request{IdempotencyKey:"idem-1",SubjectID:"booking-1"}).Validate(); err!=nil { t.Fatal(err) }
}
