package main

import (
	"encoding/json"
	"io"
	"strings"
)

// Accept only the two persistently recorded event families that this gateway
// can verify against the canonical outbox table in the same transaction.
// This is a durable receipt endpoint, not an independent Academy event owner.
func validGatewayEnvelope(e event) bool {
	if e.EventID == "" || e.IdempotencyKey == "" || e.AggregateRef == "" ||
		e.AggregateVersion == 0 || len(e.Payload) == 0 || !json.Valid(e.Payload) {
		return false
	}
	if e.EventType == "booking.ledger_committed" {
		if e.SchemaVersion != "1" || e.AggregateVersion != 1 || e.Producer != "booking-ledger" {
			return false
		}
		var p payload
		dec := json.NewDecoder(strings.NewReader(string(e.Payload)))
		dec.DisallowUnknownFields()
		if dec.Decode(&p) != nil || p.BookingID == "" || p.LedgerEntryID == "" ||
			p.EconomicEventRef == "" || e.AggregateRef != "booking/"+p.BookingID ||
			e.IdempotencyKey != "booking-ledger:"+p.EconomicEventRef {
			return false
		}
		var trailing any
		return dec.Decode(&trailing) == io.EOF
	}
	// Academy events are emitted exclusively by academyEvent() in the
	// existing transaction. Reject forged envelope metadata before the
	// canonical SQL row and full payload comparison in consume().
	if !strings.HasPrefix(e.EventType, "academy.") ||
		e.SchemaVersion != "academy-event-v1" || e.Producer != "apgic-academy" ||
		!strings.HasPrefix(e.AggregateRef, "academy/") ||
		e.CorrelationID != e.EventID ||
		e.IdempotencyKey != e.EventType+"/"+e.AggregateRef+"/"+e.EventID {
		return false
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(e.Payload, &object); err != nil || len(object) == 0 {
		return false
	}
	return true
}
