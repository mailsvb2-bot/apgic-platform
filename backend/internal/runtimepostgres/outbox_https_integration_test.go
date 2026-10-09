package runtimepostgres

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/eventspine"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

// Proves the real PostgreSQL outbox keeps an event pending after an HTTPS 503,
// retries the same idempotency key and acknowledges it exactly once on HTTPS 204.
func TestEvent001HTTPSGatewayRetryAndDurableAck(t *testing.T) {
	databaseURL := os.Getenv("APGIC_OUTBOX_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("isolated outbox integration database is not configured")
	}
	store, err := Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	id, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	event := eventspine.EventEnvelope{
		EventID: id, IdempotencyKey: "event001-https:" + id, EventType: "test.delivery",
		SchemaVersion: "1", AggregateRef: "event001-https/" + aggregate, AggregateVersion: 1,
		OccurredAt: now, ProducedAt: now, Producer: "event001-integration",
		CorrelationID: "event001-https-proof", PayloadJSON: []byte(`{"test":"https-retry"}`),
	}
	tx, err := store.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureOutboxEventTx(context.Background(), tx, event); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	assertOutboxState(t, store.db, id, "PENDING", 0)

	calls := 0
	gateway := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost ||
			r.Header.Get("Idempotency-Key") != event.IdempotencyKey ||
			r.Header.Get("X-APGIC-Service-Scope") != eventspine.EventDeliveryScope ||
			r.Header.Get("Authorization") != "Bearer "+strings.Repeat("s", 32) {
			t.Error("HTTPS gateway received an invalid delivery request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if calls == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer gateway.Close()
	deliverer, err := eventspine.NewHTTPDeliverer(gateway.URL, "event001-worker", strings.Repeat("s", 32), gateway.Client())
	if err != nil {
		t.Fatal(err)
	}

	delivered, err := store.DeliverPendingOutbox(context.Background(), 100, deliverer.Deliver)
	if delivered != 0 || !errors.Is(err, eventspine.ErrDeliveryRejected) {
		t.Fatalf("failed HTTPS attempt: delivered=%d err=%v", delivered, err)
	}
	assertOutboxState(t, store.db, id, "PENDING", 1)

	delivered, err = store.DeliverPendingOutbox(context.Background(), 100, deliverer.Deliver)
	if delivered != 1 || err != nil {
		t.Fatalf("HTTPS retry: delivered=%d err=%v", delivered, err)
	}
	assertOutboxState(t, store.db, id, "DELIVERED", 2)
	delivered, err = store.DeliverPendingOutbox(context.Background(), 100, deliverer.Deliver)
	if delivered != 0 || err != nil || calls != 2 {
		t.Fatalf("unexpected duplicate delivery: delivered=%d calls=%d err=%v", delivered, calls, err)
	}
}
