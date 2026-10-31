package runtimepostgres

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/instant"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

// HandoffSlots reads only published R2 canonical slots owned by this signed
// client's accepted Duty quote. It does not promise room availability.
func (c *Checker) HandoffSlots(dispatchID, identityID string) ([]instant.CanonicalSlot, error) {
	if !instant.ValidID(dispatchID) || !instant.ValidID(identityID) {
		return nil, instant.ErrInvalid
	}
	ctx, cancel, err := c.instantContext()
	if err != nil {
		return nil, err
	}
	defer cancel()
	if err := c.expireInstant(ctx); err != nil {
		return nil, fmt.Errorf("expire duty before slot handoff: %w", err)
	}
	var ownerID string
	err = c.db.QueryRowContext(ctx,
		"SELECT id::text FROM instant_dispatches WHERE id=$1::uuid AND client_identity_id=$2::uuid",
		dispatchID, identityID).Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, instant.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("verify duty handoff owner: %w", err)
	}
	rows, err := c.db.QueryContext(ctx, `
SELECT s.id::text,s.starts_at,s.ends_at FROM instant_dispatches d
JOIN booking_slots s ON s.specialist_identity_id=d.specialist_identity_id
 AND s.specialist_offer_id=d.offer_id AND s.tenant_scope='specialist/live'
WHERE d.id=$1::uuid AND d.client_identity_id=$2::uuid
  AND d.state='ACCEPTED' AND d.expires_at>clock_timestamp()
  AND s.exclusive=true
  AND s.starts_at>clock_timestamp()+interval '5 minutes'
  AND s.starts_at<=clock_timestamp()+interval '30 minutes'
  AND apgic_live_offer_valid(d.offer_id)
  AND NOT EXISTS (SELECT 1 FROM instant_booking_links link WHERE link.dispatch_id=d.id)
  AND NOT EXISTS (
    SELECT 1 FROM booking_holds h WHERE h.slot_id=s.id
    AND h.state='ACTIVE' AND h.expires_at>clock_timestamp()
  )
  AND NOT EXISTS (
    SELECT 1 FROM bookings b WHERE b.slot_id=s.id
      AND b.state IN ('HELD','PENDING_PAYMENT','CONFIRMED')
  )
ORDER BY s.starts_at,s.id LIMIT 20`, dispatchID, identityID)
	if err != nil {
		return nil, fmt.Errorf("list R2 handoff slots: %w", err)
	}
	defer rows.Close()
	items := make([]instant.CanonicalSlot, 0)
	for rows.Next() {
		var slot instant.CanonicalSlot
		if err := rows.Scan(&slot.ID, &slot.StartsAt, &slot.EndsAt); err != nil {
			return nil, fmt.Errorf("scan R2 handoff slot: %w", err)
		}
		items = append(items, slot)
	}
	return items, rows.Err()
}

// PrepareCanonicalBooking invokes only the existing R2 PostgreSQL owners.
// Any SQL write failure rolls back duty release, hold and booking together.
func (c *Checker) PrepareCanonicalBooking(dispatchID, identityID, slotID string) (instant.Fulfillment, error) {
	if !instant.ValidID(dispatchID) || !instant.ValidID(identityID) || !instant.ValidID(slotID) {
		return instant.Fulfillment{}, instant.ErrInvalid
	}
	ctx, cancel, err := c.instantContext()
	if err != nil {
		return instant.Fulfillment{}, err
	}
	defer cancel()
	holdID, err := persistentid.New()
	if err != nil {
		return instant.Fulfillment{}, err
	}
	bookingID, err := persistentid.New()
	if err != nil {
		return instant.Fulfillment{}, err
	}
	var reason string
	err = c.db.QueryRowContext(ctx, `SELECT apgic_instant_prepare_canonical_booking(
  $1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6)`,
		dispatchID, identityID, slotID, holdID, bookingID, time.Now().UTC()).Scan(&reason)
	if err != nil {
		return instant.Fulfillment{}, fmt.Errorf("prepare canonical R2 booking: %w", err)
	}
	switch reason {
	case "DUTY_BOOKING_LINKED", "DUTY_BOOKING_ALREADY_LINKED":
		return c.ReadFulfillment(dispatchID, identityID)
	case "DUTY_NOT_FOUND":
		return instant.Fulfillment{}, instant.ErrNotFound
	case "DUTY_HANDOFF_NOT_ELIGIBLE":
		return instant.Fulfillment{}, instant.ErrNotEligible
	case "DUTY_HANDOFF_INACTIVE", "DUTY_HANDOFF_SLOT_UNAVAILABLE", "DUTY_HANDOFF_CONFLICT":
		return instant.Fulfillment{}, instant.ErrConflict
	default:
		return instant.Fulfillment{}, fmt.Errorf("unknown canonical handoff result %q", reason)
	}
}
