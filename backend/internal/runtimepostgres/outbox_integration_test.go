package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/eventspine"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

func TestTransactionalOutboxSurvivesRollbackRestartAndRetry(t *testing.T) {
	databaseURL := os.Getenv("APGIC_OUTBOX_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("outbox integration database not configured")
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		t.Fatal(err)
	}
	store := &Checker{db: db}

	eventID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	auditID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	event := eventspine.EventEnvelope{
		EventID:          eventID,
		IdempotencyKey:   "event001:" + eventID,
		EventType:        "test.domain_effect_committed",
		SchemaVersion:    "1",
		AggregateRef:     "test/" + auditID,
		AggregateVersion: 1,
		OccurredAt:       now,
		ProducedAt:       now,
		Producer:         "event001-integration",
		CorrelationID:    "event001-recovery-proof",
		PayloadJSON:      []byte(`{"proof":"transactional-outbox"}`),
	}

	// A failed domain transaction must persist neither the business effect nor its outbox event.
	tx, err := store.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`
		INSERT INTO audit_records (
			id, actor_id, action, scope, reason, policy_version, correlation_id, occurred_at
		) VALUES ($1::uuid, 'event001-test', 'test.domain_effect', 'event001',
		          'ROLLBACK_PROOF', 'event001-v1', 'event001-rollback', $2)
	`, auditID, now); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := ensureOutboxEventTx(context.Background(), tx, event); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	assertEvent001Counts(t, store.db, auditID, eventID, 0, 0)

	// Commit the domain effect and its event atomically.
	tx, err = store.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`
		INSERT INTO audit_records (
			id, actor_id, action, scope, reason, policy_version, correlation_id, occurred_at
		) VALUES ($1::uuid, 'event001-test', 'test.domain_effect', 'event001',
		          'COMMIT_PROOF', 'event001-v1', 'event001-commit', $2)
	`, auditID, now); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := ensureOutboxEventTx(context.Background(), tx, event); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	assertEvent001Counts(t, store.db, auditID, eventID, 1, 1)

	// Simulate process loss after the DB commit but before external delivery.
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	pending, err := store.PendingOutbox(100)
	if err != nil {
		t.Fatal(err)
	}
	var recovered *eventspine.OutboxRecord
	for i := range pending {
		if pending[i].Event.EventID == eventID {
			recovered = &pending[i]
			break
		}
	}
	if recovered == nil {
		t.Fatal("committed outbox event was not recovered after store restart")
	}
	if recovered.Event.IdempotencyKey != event.IdempotencyKey || recovered.Attempts != 0 {
		t.Fatalf("unexpected recovered event: %#v", recovered)
	}

	// A failed provider attempt must leave the event pending for retry.
	expectedDeliveryErr := errors.New("simulated provider outage")
	delivered, err := store.DeliverPendingOutbox(context.Background(), 100, func(_ context.Context, candidate eventspine.EventEnvelope) error {
		if candidate.EventID == eventID {
			if candidate.IdempotencyKey != event.IdempotencyKey {
				t.Fatalf("delivery idempotency key=%q want %q", candidate.IdempotencyKey, event.IdempotencyKey)
			}
			return expectedDeliveryErr
		}
		return nil
	})
	if err == nil || !errors.Is(err, expectedDeliveryErr) {
		t.Fatalf("delivery error=%v want simulated provider outage", err)
	}
	if delivered != 0 {
		t.Fatalf("delivered=%d want 0 after provider failure", delivered)
	}
	assertOutboxState(t, store.db, eventID, "PENDING", 1)

	// Simulate another restart. Retry must use the same durable event/idempotency key.
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}

	deliveryCalls := 0
	delivered, err = store.DeliverPendingOutbox(context.Background(), 100, func(_ context.Context, candidate eventspine.EventEnvelope) error {
		if candidate.EventID != eventID {
			return nil
		}
		deliveryCalls++
		if candidate.IdempotencyKey != event.IdempotencyKey {
			t.Fatalf("retry idempotency key=%q want %q", candidate.IdempotencyKey, event.IdempotencyKey)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if delivered != 1 || deliveryCalls != 1 {
		t.Fatalf("delivered=%d deliveryCalls=%d want 1/1", delivered, deliveryCalls)
	}
	assertOutboxState(t, store.db, eventID, "DELIVERED", 2)

	// Terminal delivery must not be emitted again.
	delivered, err = store.DeliverPendingOutbox(context.Background(), 100, func(_ context.Context, candidate eventspine.EventEnvelope) error {
		if candidate.EventID == eventID {
			t.Fatal("terminal outbox event was delivered twice")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if delivered != 0 {
		t.Fatalf("second terminal delivery count=%d want 0", delivered)
	}
	assertEvent001Counts(t, store.db, auditID, eventID, 1, 1)
}

func assertEvent001Counts(t *testing.T, db *sql.DB, auditID, eventID string, wantDomain, wantOutbox int) {
	t.Helper()
	var domainCount, outboxCount int
	if err := db.QueryRow(`SELECT count(*) FROM audit_records WHERE id = $1::uuid`, auditID).Scan(&domainCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM outbox_events WHERE event_id = $1::uuid`, eventID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if domainCount != wantDomain || outboxCount != wantOutbox {
		t.Fatalf("domain/outbox counts=%d/%d want %d/%d", domainCount, outboxCount, wantDomain, wantOutbox)
	}
}

func assertOutboxState(t *testing.T, db *sql.DB, eventID, wantStatus string, wantAttempts int) {
	t.Helper()
	var status string
	var attempts int
	if err := db.QueryRow(`
		SELECT delivery_status, attempts
		  FROM outbox_events
		 WHERE event_id = $1::uuid
	`, eventID).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != wantStatus || attempts != wantAttempts {
		t.Fatalf("outbox status/attempts=%s/%d want %s/%d", status, attempts, wantStatus, wantAttempts)
	}
}
