package eventspine

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPDelivererPreservesCanonicalEnvelopeAndScopedCredential(t *testing.T) {
	var got map[string]any
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Idempotency-Key") != "idem-1" {
			t.Fatalf("idempotency header=%q", r.Header.Get("Idempotency-Key"))
		}
		if r.Header.Get("X-APGIC-Service-Principal") != "outbox-worker" ||
			r.Header.Get("X-APGIC-Service-Scope") != EventDeliveryScope {
			t.Fatalf("service identity headers=%q/%q", r.Header.Get("X-APGIC-Service-Principal"), r.Header.Get("X-APGIC-Service-Scope"))
		}
		if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("s", 32) {
			t.Fatal("gateway credential was not sent as configured")
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	deliverer, err := NewHTTPDeliverer(server.URL, "outbox-worker", strings.Repeat("s", 32), server.Client())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 9, 9, 0, 0, 0, time.UTC)
	event := EventEnvelope{
		EventID: "event-1", IdempotencyKey: "idem-1", EventType: "booking.changed",
		SchemaVersion: "1", AggregateRef: "booking/b-1", AggregateVersion: 4,
		OccurredAt: now, ProducedAt: now, Producer: "booking",
		CorrelationID: "corr-1", PayloadJSON: []byte(`{"booking_id":"b-1"}`),
	}
	if err := deliverer.Deliver(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if got["event_id"] != "event-1" || got["idempotency_key"] != "idem-1" ||
		got["aggregate_ref"] != "booking/b-1" {
		t.Fatalf("wire envelope=%#v", got)
	}
}

func TestHTTPDelivererFailsClosedOnConfigAndNon2xx(t *testing.T) {
	if _, err := NewHTTPDeliverer("http://example.test/events", "worker", strings.Repeat("s", 32), nil); !errors.Is(err, ErrDeliveryConfigInvalid) {
		t.Fatalf("plain HTTP config err=%v", err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	deliverer, err := NewHTTPDeliverer(server.URL, "worker", strings.Repeat("s", 32), server.Client())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	err = deliverer.Deliver(context.Background(), EventEnvelope{
		EventID: "event-1", IdempotencyKey: "idem-1", EventType: "test.event",
		SchemaVersion: "1", AggregateRef: "test/1", OccurredAt: now, ProducedAt: now,
		Producer: "test", CorrelationID: "corr", PayloadJSON: []byte(`{"ok":true}`),
	})
	if !errors.Is(err, ErrDeliveryRejected) {
		t.Fatalf("non-2xx err=%v", err)
	}
}

func TestHTTPDelivererDoesNotFollowGatewayRedirectWithCredentials(t *testing.T) {
	redirectCalls := 0
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectCalls++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	gateway := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer gateway.Close()
	client := gateway.Client()
	// Redirects must also stay disabled if caller supplies a permissive client.
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return nil }
	deliverer, err := NewHTTPDeliverer(gateway.URL, "worker", strings.Repeat("s", 32), client)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	err = deliverer.Deliver(context.Background(), EventEnvelope{
		EventID: "event-1", IdempotencyKey: "idem-1", EventType: "test.event",
		SchemaVersion: "1", AggregateRef: "test/1", OccurredAt: now, ProducedAt: now,
		Producer: "test", CorrelationID: "corr", PayloadJSON: []byte(`{"ok":true}`),
	})
	if !errors.Is(err, ErrDeliveryRejected) {
		t.Fatalf("redirect err=%v", err)
	}
	if redirectCalls != 0 {
		t.Fatalf("redirect target received %d requests", redirectCalls)
	}
	if client.CheckRedirect == nil {
		t.Fatal("supplied HTTP client was mutated")
	}
}

func TestOutboxPayloadMustMatchObjectContract(t *testing.T) {
	now := time.Now().UTC()
	base := EventEnvelope{
		EventID: "event-1", IdempotencyKey: "idem-1", EventType: "test.event",
		SchemaVersion: "1", AggregateRef: "test/1", OccurredAt: now,
		Producer: "test", CorrelationID: "corr",
	}
	for _, invalid := range []string{`[]`, `"text"`, `null`} {
		candidate := base
		candidate.PayloadJSON = []byte(invalid)
		if _, err := NewRecord(candidate); !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("payload %s must violate object contract, err=%v", invalid, err)
		}
	}
}
