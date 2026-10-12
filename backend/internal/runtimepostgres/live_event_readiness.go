package runtimepostgres

import (
	"context"
	"fmt"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/liveevents"
)

// LiveEventsReady prevents the R9 feature flag from making a deployment appear
// healthy before its tenant, registration, attendance and migration guards exist.
func (c *Checker) LiveEventsReady(ctx context.Context) error {
	if c == nil || c.db == nil {
		return liveevents.ErrUnavailable
	}
	var migrated bool
	err := c.db.QueryRowContext(ctx, `SELECT
  to_regclass('public.live_events') IS NOT NULL
  AND to_regclass('public.live_event_registrations') IS NOT NULL
  AND to_regclass('public.live_event_attendance_facts') IS NOT NULL
  AND to_regclass('public.live_event_mutations') IS NOT NULL
  AND to_regprocedure('public.apgic_r9_event_guard()') IS NOT NULL
  AND to_regprocedure('public.apgic_r9_registration_guard()') IS NOT NULL
  AND to_regprocedure('public.apgic_r9_attendance_guard()') IS NOT NULL
`).Scan(&migrated)
	if err != nil {
		return fmt.Errorf("R9 live events readiness query: %w", err)
	}
	if !migrated {
		return fmt.Errorf("R9 live events schema/guards missing: %w", liveevents.ErrUnavailable)
	}
	return nil
}
