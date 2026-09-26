package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
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

func (c *Checker) Close() error {
	if c == nil || c.db == nil {
		return nil
	}
	return c.db.Close()
}
