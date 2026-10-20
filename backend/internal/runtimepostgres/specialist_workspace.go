package runtimepostgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/specialist"
)

// SpecialistWorkspace reads actual slots, bookings and provider session states.
// Ownership is checked by the trusted session identity inside the SQL query.
func (c *Checker) SpecialistWorkspace(ctx context.Context, identityID string, from, until time.Time) ([]specialist.WorkspaceEntry, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("postgres checker is not initialized")
	}
	if !until.After(from) || until.Sub(from) > 370*24*time.Hour {
		return nil, specialist.ErrPublicDetailsInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, specialistWriteTimeout)
	defer cancel()
	rows, err := c.db.QueryContext(ctx, `
SELECT slot.id::text, COALESCE(booking.id::text,''), COALESCE(booking.state,''),
       COALESCE(session.state,''), slot.starts_at, slot.ends_at, COALESCE(offer.topic_id,'')
FROM booking_slots slot
JOIN specialist_profiles profile ON profile.identity_id=slot.specialist_identity_id
LEFT JOIN specialist_bookable_offers offer ON offer.id=slot.specialist_offer_id
LEFT JOIN LATERAL (
 SELECT b.id,b.state FROM bookings b WHERE b.slot_id=slot.id
 ORDER BY CASE WHEN b.state='COMPLETED' THEN 0 WHEN b.state='CONFIRMED' THEN 1 ELSE 2 END,
          b.starts_at DESC,b.id DESC LIMIT 1
) booking ON true
LEFT JOIN consultation_sessions session ON session.booking_id=booking.id
WHERE profile.identity_id=$1::uuid AND slot.tenant_scope='specialist/live'
  AND slot.starts_at >= $2 AND slot.starts_at < $3
ORDER BY slot.starts_at ASC,slot.id ASC LIMIT 500`, identityID, from.UTC(), until.UTC())
	if err != nil {
		return nil, fmt.Errorf("read specialist workspace: %w", err)
	}
	defer rows.Close()
	entries := make([]specialist.WorkspaceEntry, 0)
	for rows.Next() {
		var entry specialist.WorkspaceEntry
		if err := rows.Scan(&entry.SlotID, &entry.BookingID, &entry.BookingState,
			&entry.ConsultationState, &entry.StartsAt, &entry.EndsAt, &entry.TopicID); err != nil {
			return nil, fmt.Errorf("scan specialist booking: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate specialist bookings: %w", err)
	}
	return entries, nil
}
