package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/audit"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/ledger"
)

const ReasonStorageUnavailable = "STORAGE_UNAVAILABLE"

var requiredTables = []string{
	"identities",
	"identity_roles",
	"help_intents",
	"outbox_events",
	"audit_records",
	"ledger_entries",
	"booking_slots",
	"booking_holds",
	"bookings",
	"specialist_profiles",
	"specialist_capabilities",
	"specialist_evidence",
	"qualification_evaluations",
	"specialist_publications",
	"organization_ownerships",
}

var requiredIndexes = []string{
	"ledger_entries_economic_event_ref_unique",
}

type Checker struct {
	db *sql.DB
}

func RequiresDatabase(environment string) bool {
	switch strings.ToUpper(strings.TrimSpace(environment)) {
	case "STAGING", "PRODUCTION":
		return true
	default:
		return false
	}
}

func Open(ctx context.Context, databaseURL string) (*Checker, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, errors.New("APGIC_DATABASE_URL is required")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	checker := &Checker{db: db}
	if err := checker.Ready(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return checker, nil
}

func (c *Checker) Ready(ctx context.Context) error {
	if c == nil || c.db == nil {
		return errors.New("postgres checker is not initialized")
	}
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := c.db.PingContext(probeCtx); err != nil {
		return fmt.Errorf("postgres ping: %w", err)
	}
	for _, table := range requiredTables {
		var exists bool
		err := c.db.QueryRowContext(
			probeCtx,
			"SELECT to_regclass($1) IS NOT NULL",
			"public."+table,
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("probe %s: %w", table, err)
		}
		if !exists {
			return fmt.Errorf("required table missing: %s", table)
		}
	}
	for _, index := range requiredIndexes {
		var exists bool
		err := c.db.QueryRowContext(
			probeCtx,
			"SELECT to_regclass($1) IS NOT NULL",
			"public."+index,
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("probe %s: %w", index, err)
		}
		if !exists {
			return fmt.Errorf("required index missing: %s", index)
		}
	}
	return nil
}

func (c *Checker) Append(record audit.Record) error {
	if c == nil || c.db == nil {
		return errors.New("postgres checker is not initialized")
	}
	canonical, err := audit.New(record)
	if err != nil {
		return err
	}

	writeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := c.db.ExecContext(
		writeCtx,
		`INSERT INTO audit_records (
			id, actor_id, action, scope, resource_ref, old_state, new_state,
			reason, policy_version, correlation_id, occurred_at
		) VALUES (
			$1::uuid, $2, $3, $4, NULLIF($5, ''), $6::jsonb, $7::jsonb,
			$8, $9, NULLIF($10, ''), $11
		)`,
		canonical.ID,
		canonical.ActorID,
		canonical.Action,
		canonical.Scope,
		canonical.ResourceRef,
		nullableJSON(canonical.OldState),
		nullableJSON(canonical.NewState),
		canonical.Reason,
		canonical.PolicyVersion,
		canonical.CorrelationID,
		canonical.OccurredAt,
	); err != nil {
		return fmt.Errorf("append audit record: %w", err)
	}
	return nil
}

func nullableJSON(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return string(value)
}

func (c *Checker) AppendLedgerEntry(entry ledger.Entry) (ledger.Entry, error) {
	if c == nil || c.db == nil {
		return ledger.Entry{}, errors.New("postgres checker is not initialized")
	}
	canonical, err := ledger.NewEntry(entry)
	if err != nil {
		return ledger.Entry{}, err
	}
	if strings.TrimSpace(canonical.EconomicEventRef) == "" {
		return ledger.Entry{}, errors.New("ledger economic_event_ref is required for durable idempotency")
	}

	writeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var insertedID string
	err = c.db.QueryRowContext(
		writeCtx,
		`INSERT INTO ledger_entries (
			id, debit_account_ref, credit_account_ref, amount_minor, currency,
			provider_evidence_ref, economic_event_ref, correlation_id, occurred_at
		) VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (economic_event_ref) WHERE economic_event_ref IS NOT NULL
		DO NOTHING
		RETURNING id::text`,
		canonical.ID,
		canonical.DebitAccountRef,
		canonical.CreditAccountRef,
		canonical.AmountMinor,
		canonical.Currency,
		canonical.ProviderEvidenceRef,
		canonical.EconomicEventRef,
		canonical.CorrelationID,
		canonical.OccurredAt,
	).Scan(&insertedID)
	if err == nil {
		canonical.ID = insertedID
		return canonical, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return ledger.Entry{}, fmt.Errorf("append ledger entry: %w", err)
	}

	existing, err := c.ledgerEntryByEconomicEvent(writeCtx, canonical.EconomicEventRef)
	if err != nil {
		return ledger.Entry{}, fmt.Errorf("read idempotent ledger entry: %w", err)
	}
	if !sameEconomicEffect(existing, canonical) {
		return ledger.Entry{}, fmt.Errorf("ledger economic event collision: %s", canonical.EconomicEventRef)
	}
	return existing, nil
}

func (c *Checker) CommitBookingLedger(booked *booking.Booking, entry ledger.Entry) (ledger.Entry, bool, error) {
	if c == nil || c.db == nil {
		return ledger.Entry{}, false, errors.New("postgres checker is not initialized")
	}
	if booked == nil || booked.UpdatedAt.IsZero() {
		return ledger.Entry{}, false, errors.New("booking state and updated_at are required")
	}
	canonical, err := ledger.NewEntry(entry)
	if err != nil {
		return ledger.Entry{}, false, err
	}
	if strings.TrimSpace(canonical.EconomicEventRef) == "" {
		return ledger.Entry{}, false, errors.New("ledger economic_event_ref is required for durable idempotency")
	}

	writeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tx, err := c.db.BeginTx(writeCtx, nil)
	if err != nil {
		return ledger.Entry{}, false, fmt.Errorf("begin atomic booking ledger commit: %w", err)
	}
	defer tx.Rollback()

	var insertedID string
	inserted := true
	err = tx.QueryRowContext(
		writeCtx,
		`INSERT INTO ledger_entries (
			id, debit_account_ref, credit_account_ref, amount_minor, currency,
			provider_evidence_ref, economic_event_ref, correlation_id, occurred_at
		) VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (economic_event_ref) WHERE economic_event_ref IS NOT NULL
		DO NOTHING
		RETURNING id::text`,
		canonical.ID,
		canonical.DebitAccountRef,
		canonical.CreditAccountRef,
		canonical.AmountMinor,
		canonical.Currency,
		canonical.ProviderEvidenceRef,
		canonical.EconomicEventRef,
		canonical.CorrelationID,
		canonical.OccurredAt,
	).Scan(&insertedID)
	if err == nil {
		canonical.ID = insertedID
	} else if !errors.Is(err, sql.ErrNoRows) {
		return ledger.Entry{}, false, fmt.Errorf("append atomic ledger entry: %w", err)
	} else {
		inserted = false
		var existing ledger.Entry
		var economicEvent sql.NullString
		if err := tx.QueryRowContext(
			writeCtx,
			`SELECT
				id::text, debit_account_ref, credit_account_ref, amount_minor, currency,
				provider_evidence_ref, economic_event_ref, correlation_id, occurred_at
			FROM ledger_entries
			WHERE economic_event_ref = $1`,
			canonical.EconomicEventRef,
		).Scan(
			&existing.ID,
			&existing.DebitAccountRef,
			&existing.CreditAccountRef,
			&existing.AmountMinor,
			&existing.Currency,
			&existing.ProviderEvidenceRef,
			&economicEvent,
			&existing.CorrelationID,
			&existing.OccurredAt,
		); err != nil {
			return ledger.Entry{}, false, fmt.Errorf("read idempotent atomic ledger entry: %w", err)
		}
		if economicEvent.Valid {
			existing.EconomicEventRef = economicEvent.String
		}
		if !sameEconomicEffect(existing, canonical) {
			return ledger.Entry{}, false, fmt.Errorf("ledger economic event collision: %s", canonical.EconomicEventRef)
		}
		canonical = existing
	}
	if !inserted {
		var currentState string
		if err := tx.QueryRowContext(
			writeCtx,
			`SELECT state FROM bookings WHERE id = $1::uuid`,
			booked.ID,
		).Scan(&currentState); err != nil {
			return ledger.Entry{}, false, fmt.Errorf("read idempotent booking state: %w", err)
		}
		if currentState != string(booked.State) {
			return ledger.Entry{}, false, fmt.Errorf(
				"ledger effect exists but booking state is %s, expected %s",
				currentState,
				booked.State,
			)
		}
		outboxEvent, err := bookingLedgerOutboxEvent(booked, canonical)
		if err != nil {
			return ledger.Entry{}, false, err
		}
		if err := ensureOutboxEventTx(writeCtx, tx, outboxEvent); err != nil {
			return ledger.Entry{}, false, fmt.Errorf("ensure idempotent booking outbox event: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return ledger.Entry{}, false, fmt.Errorf("commit idempotent booking ledger replay: %w", err)
		}
		return canonical, true, nil
	}

	result, err := tx.ExecContext(
		writeCtx,
		`UPDATE bookings
		 SET state = $2,
		     updated_at = $3
		 WHERE id = $1::uuid`,
		booked.ID,
		string(booked.State),
		booked.UpdatedAt,
	)
	if err != nil {
		return ledger.Entry{}, false, fmt.Errorf("update booking in atomic ledger commit: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return ledger.Entry{}, false, fmt.Errorf("read booking update rows: %w", err)
	}
	if affected != 1 {
		return ledger.Entry{}, false, fmt.Errorf("atomic booking update affected %d rows", affected)
	}
	outboxEvent, err := bookingLedgerOutboxEvent(booked, canonical)
	if err != nil {
		return ledger.Entry{}, false, err
	}
	if err := ensureOutboxEventTx(writeCtx, tx, outboxEvent); err != nil {
		return ledger.Entry{}, false, fmt.Errorf("persist atomic booking outbox event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ledger.Entry{}, false, fmt.Errorf("commit atomic booking ledger effect: %w", err)
	}
	return canonical, false, nil
}

func (c *Checker) ledgerEntryByEconomicEvent(ctx context.Context, economicEventRef string) (ledger.Entry, error) {
	var entry ledger.Entry
	var economicEvent sql.NullString
	err := c.db.QueryRowContext(
		ctx,
		`SELECT
			id::text, debit_account_ref, credit_account_ref, amount_minor, currency,
			provider_evidence_ref, economic_event_ref, correlation_id, occurred_at
		FROM ledger_entries
		WHERE economic_event_ref = $1`,
		economicEventRef,
	).Scan(
		&entry.ID,
		&entry.DebitAccountRef,
		&entry.CreditAccountRef,
		&entry.AmountMinor,
		&entry.Currency,
		&entry.ProviderEvidenceRef,
		&economicEvent,
		&entry.CorrelationID,
		&entry.OccurredAt,
	)
	if economicEvent.Valid {
		entry.EconomicEventRef = economicEvent.String
	}
	return entry, err
}

func sameEconomicEffect(left, right ledger.Entry) bool {
	return left.DebitAccountRef == right.DebitAccountRef &&
		left.CreditAccountRef == right.CreditAccountRef &&
		left.AmountMinor == right.AmountMinor &&
		left.Currency == right.Currency &&
		left.ProviderEvidenceRef == right.ProviderEvidenceRef &&
		left.EconomicEventRef == right.EconomicEventRef &&
		left.CorrelationID == right.CorrelationID
}

func (c *Checker) LedgerEntries() ([]ledger.Entry, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("postgres checker is not initialized")
	}
	readCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rows, err := c.db.QueryContext(
		readCtx,
		`SELECT
			id::text, debit_account_ref, credit_account_ref, amount_minor, currency,
			provider_evidence_ref, economic_event_ref, correlation_id, occurred_at
		FROM ledger_entries
		ORDER BY occurred_at, id`,
	)
	if err != nil {
		return nil, fmt.Errorf("read ledger entries: %w", err)
	}
	defer rows.Close()

	entries := make([]ledger.Entry, 0)
	for rows.Next() {
		var entry ledger.Entry
		var economicEvent sql.NullString
		if err := rows.Scan(
			&entry.ID,
			&entry.DebitAccountRef,
			&entry.CreditAccountRef,
			&entry.AmountMinor,
			&entry.Currency,
			&entry.ProviderEvidenceRef,
			&economicEvent,
			&entry.CorrelationID,
			&entry.OccurredAt,
		); err != nil {
			return nil, fmt.Errorf("scan ledger entry: %w", err)
		}
		if economicEvent.Valid {
			entry.EconomicEventRef = economicEvent.String
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read ledger entries: %w", err)
	}
	return entries, nil
}

func (c *Checker) Close() error {
	if c == nil || c.db == nil {
		return nil
	}
	return c.db.Close()
}
