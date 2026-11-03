package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestGatewayAcademyCanonicalOutboxEnvelope(t *testing.T) {
	id := "34746d83-fa08-458a-9750-1466083aea71"
	ref := "academy/course/8dfd43b4-b565-4290-8a3e-7b58cb09a88c"
	e := event{
		EventID: id,
		IdempotencyKey: "academy.course.created/" + ref + "/" + id,
		EventType: "academy.course.created",
		SchemaVersion: "academy-event-v1",
		AggregateRef: ref,
		AggregateVersion: 1,
		OccurredAt: time.Now().UTC(),
		ProducedAt: time.Now().UTC(),
		Producer: "apgic-academy",
		CorrelationID: id,
		Payload: json.RawMessage(`{"author_id":"00000000-0000-4000-8000-000000000101","version_id":"00000000-0000-4000-8000-000000000102"}`),
	}
	if !validGatewayEnvelope(e) {
		t.Fatal("canonical persisted Academy course event rejected")
	}
	m := e
	m.EventType = "academy.module.added"
	m.IdempotencyKey = m.EventType + "/" + m.AggregateRef + "/" + m.EventID
	m.AggregateRef = "academy/version/0bc2d8cf-fe32-4e8d-99fb-e48a17fec90f"
	m.IdempotencyKey = m.EventType + "/" + m.AggregateRef + "/" + m.EventID
	if !validGatewayEnvelope(m) {
		t.Fatal("canonical Academy module event rejected")
	}
	for _, tc := range []struct {
		name string
		change func(*event)
	}{
		{"foreign producer",func(e *event){e.Producer="foreign"}},
		{"wrong schema",func(e *event){e.SchemaVersion="1"}},
		{"wrong event family",func(e *event){e.EventType="finance.payout_executed"}},
		{"wrong aggregate",func(e *event){e.AggregateRef="booking/foreign"}},
		{"altered correlation",func(e *event){e.CorrelationID="foreign"}},
		{"altered idempotency",func(e *event){e.IdempotencyKey="foreign"}},
		{"zero version",func(e *event){e.AggregateVersion=0}},
		{"empty payload",func(e *event){e.Payload=nil}},
		{"array payload",func(e *event){e.Payload=json.RawMessage(`[1,2]`)}},
		{"null payload",func(e *event){e.Payload=json.RawMessage(`null`)}},
	} {
		t.Run(tc.name,func(t *testing.T){
			bad:=e
			tc.change(&bad)
			if validGatewayEnvelope(bad) {
				t.Fatal("invalid Academy event accepted")
			}
		})
	}
}
