package runtimepostgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/eventspine"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/ledger"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

const outboxDeliveryTimeout = 10 * time.Second

type OutboxDeliverer func(context.Context, eventspine.EventEnvelope) error

func bookingLedgerOutboxEvent(booked *booking.Booking, entry ledger.Entry) (eventspine.EventEnvelope, error) {
	if booked == nil {
		return eventspine.EventEnvelope{}, errors.New("booking is required for outbox event")
	}
	eventID, err := persistentid.FromRef("outbox-booking-ledger", entry.EconomicEventRef)
	if err != nil {
		return eventspine.EventEnvelope{}, err
	}
	payload, err := json.Marshal(map[string]any{
		"booking_id":         booked.ID,
		"booking_state":      string(booked.State),
		"ledger_entry_id":    entry.ID,
		"economic_event_ref": entry.EconomicEventRef,
	})
	if err != nil {
		return eventspine.EventEnvelope{}, fmt.Errorf("encode booking ledger outbox payload: %w", err)
	}
	return eventspine.EventEnvelope{
		EventID:          eventID,
		IdempotencyKey:   "booking-ledger:" + entry.EconomicEventRef,
		EventType:        "booking.ledger_committed",
		SchemaVersion:    "1",
		AggregateRef:     "booking/" + booked.ID,
		AggregateVersion: 1,
		OccurredAt:       entry.OccurredAt,
		ProducedAt:       entry.OccurredAt,
		Producer:         "booking-ledger",
		CorrelationID:    entry.CorrelationID,
		CausationID:      entry.ProviderEvidenceRef,
		PayloadJSON:      payload,
	}, nil
}

func ensureOutboxEventTx(ctx context.Context, tx *sql.Tx, event eventspine.EventEnvelope) error {
	record, err := eventspine.NewRecord(event)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx,
		`INSERT INTO outbox_events (
			event_id, idempotency_key, event_type, schema_version, aggregate_ref,
			aggregate_version, tenant_scope, correlation_id, causation_id,
			occurred_at, produced_at, producer, payload, delivery_status, attempts
		) VALUES (
			$1::uuid, $2, $3, $4, $5, $6, NULLIF($7, ''), $8, NULLIF($9, ''),
			$10, $11, $12, $13::jsonb, 'PENDING', 0
		)
		ON CONFLICT (idempotency_key) DO NOTHING`,
		record.Event.EventID,
		record.Event.IdempotencyKey,
		record.Event.EventType,
		record.Event.SchemaVersion,
		record.Event.AggregateRef,
		record.Event.AggregateVersion,
		record.Event.TenantScope,
		record.Event.CorrelationID,
		record.Event.CausationID,
		record.Event.OccurredAt,
		record.Event.ProducedAt,
		record.Event.Producer,
		string(record.Event.PayloadJSON),
	)
	if err != nil {
		return fmt.Errorf("persist outbox event: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read outbox insert rows: %w", err)
	}
	if affected == 1 {
		return nil
	}

	existing, err := outboxEventByIdempotencyKeyTx(ctx, tx, record.Event.IdempotencyKey)
	if err != nil {
		return fmt.Errorf("read idempotent outbox event: %w", err)
	}
	if !sameOutboxEvent(existing.Event, record.Event) {
		return fmt.Errorf("outbox idempotency collision: %s", record.Event.IdempotencyKey)
	}
	return nil
}

func (c *Checker) PendingOutbox(limit int) ([]eventspine.OutboxRecord, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("postgres checker is not initialized")
	}
	if limit <= 0 || limit > 1000 {
		return nil, errors.New("outbox limit must be within 1..1000")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	rows, err := c.db.QueryContext(ctx,
		`SELECT event_id::text, idempotency_key, event_type, schema_version, aggregate_ref,
		        aggregate_version, tenant_scope, correlation_id, causation_id,
		        occurred_at, produced_at, producer, payload::text,
		        delivery_status, attempts, delivered_at
		   FROM outbox_events
		  WHERE delivery_status = 'PENDING'
		  ORDER BY produced_at, event_id
		  LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("read pending outbox: %w", err)
	}
	defer rows.Close()

	records := make([]eventspine.OutboxRecord, 0)
	for rows.Next() {
		record, err := scanOutboxRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending outbox: %w", err)
	}
	return records, nil
}

func (c *Checker) DeliverPendingOutbox(ctx context.Context, limit int, deliver OutboxDeliverer) (int, error) {
	if c == nil || c.db == nil {
		return 0, errors.New("postgres checker is not initialized")
	}
	if deliver == nil {
		return 0, errors.New("outbox deliverer is required")
	}
	pending, err := c.PendingOutbox(limit)
	if err != nil {
		return 0, err
	}
	delivered := 0
	for _, candidate := range pending {
		ok, err := c.deliverOneOutbox(ctx, candidate.Event.EventID, deliver)
		if err != nil {
			return delivered, err
		}
		if ok {
			delivered++
		}
	}
	return delivered, nil
}

func (c *Checker) deliverOneOutbox(parent context.Context, eventID string, deliver OutboxDeliverer) (bool, error) {
	ctx, cancel := context.WithTimeout(parent, outboxDeliveryTimeout)
	defer cancel()

	conn, err := c.db.Conn(ctx)
	if err != nil {
		return false, fmt.Errorf("acquire outbox connection: %w", err)
	}
	defer conn.Close()

	lockKey := "apgic:outbox:" + eventID
	var locked bool
	if err := conn.QueryRowContext(ctx,
		`SELECT pg_try_advisory_lock(hashtextextended($1, 0))`,
		lockKey,
	).Scan(&locked); err != nil {
		return false, fmt.Errorf("lock outbox event: %w", err)
	}
	if !locked {
		return false, nil
	}
	defer func() {
		unlockCtx, unlockCancel := context.WithTimeout(context.Background(), time.Second)
		defer unlockCancel()
		var ignored bool
		_ = conn.QueryRowContext(unlockCtx,
			`SELECT pg_advisory_unlock(hashtextextended($1, 0))`,
			lockKey,
		).Scan(&ignored)
	}()

	var status string
	if err := conn.QueryRowContext(ctx,
		`SELECT delivery_status
		   FROM outbox_events
		  WHERE event_id = $1::uuid`,
		eventID,
	).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("read locked outbox status: %w", err)
	}
	if status != string(eventspine.Pending) {
		return false, nil
	}

	record, err := outboxEventByIDConn(ctx, conn, eventID)
	if err != nil {
		return false, err
	}
	attemptResult, err := conn.ExecContext(ctx,
		`UPDATE outbox_events
		    SET attempts = attempts + 1
		  WHERE event_id = $1::uuid
		    AND delivery_status = 'PENDING'`,
		eventID,
	)
	if err != nil {
		return false, fmt.Errorf("increment outbox attempts: %w", err)
	}
	attemptRows, err := attemptResult.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read outbox attempt rows: %w", err)
	}
	if attemptRows != 1 {
		return false, nil
	}

	if err := deliver(ctx, record.Event); err != nil {
		return false, fmt.Errorf("deliver outbox event %s: %w", eventID, err)
	}

	deliveredAt := time.Now().UTC()
	result, err := conn.ExecContext(ctx,
		`UPDATE outbox_events
		    SET delivery_status = 'DELIVERED',
		        delivered_at = $2
		  WHERE event_id = $1::uuid
		    AND delivery_status = 'PENDING'`,
		eventID,
		deliveredAt,
	)
	if err != nil {
		return false, fmt.Errorf("mark outbox delivered: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read outbox delivery rows: %w", err)
	}
	if affected != 1 {
		return false, errors.New("outbox event lost pending state before acknowledgement")
	}
	return true, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanOutboxRecord(row rowScanner) (eventspine.OutboxRecord, error) {
	var record eventspine.OutboxRecord
	var tenantScope, causationID sql.NullString
	var payload string
	var status string
	var deliveredAt sql.NullTime
	var aggregateVersion int64
	var attempts int64
	if err := row.Scan(
		&record.Event.EventID,
		&record.Event.IdempotencyKey,
		&record.Event.EventType,
		&record.Event.SchemaVersion,
		&record.Event.AggregateRef,
		&aggregateVersion,
		&tenantScope,
		&record.Event.CorrelationID,
		&causationID,
		&record.Event.OccurredAt,
		&record.Event.ProducedAt,
		&record.Event.Producer,
		&payload,
		&status,
		&attempts,
		&deliveredAt,
	); err != nil {
		return eventspine.OutboxRecord{}, fmt.Errorf("scan outbox record: %w", err)
	}
	if aggregateVersion < 0 || attempts < 0 {
		return eventspine.OutboxRecord{}, errors.New("durable outbox counters cannot be negative")
	}
	record.Event.AggregateVersion = uint64(aggregateVersion)
	record.Attempts = uint32(attempts)
	if tenantScope.Valid {
		record.Event.TenantScope = tenantScope.String
	}
	if causationID.Valid {
		record.Event.CausationID = causationID.String
	}
	record.Event.PayloadJSON = []byte(payload)
	record.Status = eventspine.DeliveryStatus(status)
	if deliveredAt.Valid {
		value := deliveredAt.Time
		record.DeliveredAt = &value
	}
	if _, err := eventspine.NewRecord(record.Event); err != nil {
		return eventspine.OutboxRecord{}, fmt.Errorf("invalid durable outbox event: %w", err)
	}
	return record, nil
}

func outboxEventByIdempotencyKeyTx(ctx context.Context, tx *sql.Tx, key string) (eventspine.OutboxRecord, error) {
	return queryOutboxRecord(tx.QueryRowContext(ctx,
		`SELECT event_id::text, idempotency_key, event_type, schema_version, aggregate_ref,
		        aggregate_version, tenant_scope, correlation_id, causation_id,
		        occurred_at, produced_at, producer, payload::text,
		        delivery_status, attempts, delivered_at
		   FROM outbox_events
		  WHERE idempotency_key = $1`,
		key,
	))
}

func outboxEventByIDConn(ctx context.Context, conn *sql.Conn, eventID string) (eventspine.OutboxRecord, error) {
	return queryOutboxRecord(conn.QueryRowContext(ctx,
		`SELECT event_id::text, idempotency_key, event_type, schema_version, aggregate_ref,
		        aggregate_version, tenant_scope, correlation_id, causation_id,
		        occurred_at, produced_at, producer, payload::text,
		        delivery_status, attempts, delivered_at
		   FROM outbox_events
		  WHERE event_id = $1::uuid`,
		eventID,
	))
}

func queryOutboxRecord(row *sql.Row) (eventspine.OutboxRecord, error) {
	record, err := scanOutboxRecord(row)
	if err != nil {
		return eventspine.OutboxRecord{}, err
	}
	return record, nil
}

func sameOutboxEvent(left, right eventspine.EventEnvelope) bool {
	return left.EventID == right.EventID &&
		left.IdempotencyKey == right.IdempotencyKey &&
		left.EventType == right.EventType &&
		left.SchemaVersion == right.SchemaVersion &&
		left.AggregateRef == right.AggregateRef &&
		left.AggregateVersion == right.AggregateVersion &&
		strings.TrimSpace(left.TenantScope) == strings.TrimSpace(right.TenantScope) &&
		left.CorrelationID == right.CorrelationID &&
		left.CausationID == right.CausationID &&
		left.OccurredAt.Equal(right.OccurredAt) &&
		left.ProducedAt.Equal(right.ProducedAt) &&
		left.Producer == right.Producer &&
		sameJSON(left.PayloadJSON, right.PayloadJSON)
}

func sameJSON(left, right []byte) bool {
	var leftValue, rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}
