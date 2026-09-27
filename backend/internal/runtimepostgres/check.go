package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/ledger"
)

const ReasonStorageUnavailable = "STORAGE_UNAVAILABLE"

var requiredTables = []string{
	"identities",
	"outbox_events",
	"audit_records",
	"ledger_entries",
	"booking_slots",
	"booking_holds",
	"bookings",
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
	return nil
}

func (c *Checker) AppendLedgerEntry(entry ledger.Entry) error {
	if c == nil || c.db == nil {
		return errors.New("postgres checker is not initialized")
	}
	canonical, err := ledger.NewEntry(entry)
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err = c.db.ExecContext(
		writeCtx,
		`INSERT INTO ledger_entries (
			id, debit_account_ref, credit_account_ref, amount_minor, currency,
			provider_evidence_ref, economic_event_ref, correlation_id, occurred_at
		) VALUES ($1::uuid, $2, $3, $4, $5, $6, NULLIF($7, ''), $8, $9)`,
		canonical.ID,
		canonical.DebitAccountRef,
		canonical.CreditAccountRef,
		canonical.AmountMinor,
		canonical.Currency,
		canonical.ProviderEvidenceRef,
		canonical.EconomicEventRef,
		canonical.CorrelationID,
		canonical.OccurredAt,
	)
	if err != nil {
		return fmt.Errorf("append ledger entry: %w", err)
	}
	return nil
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
