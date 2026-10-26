package runtimepostgres

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/instant"
)

// ReadFulfillment never trusts the browser's booking reference and never
// authorizes a room. Only the signed client can read its dispatch.
func (c *Checker) ReadFulfillment(dispatchID, identityID string) (instant.Fulfillment, error) {
	if !instant.ValidID(dispatchID) || !instant.ValidID(identityID) {
		return instant.Fulfillment{}, instant.ErrInvalid
	}
	ctx, cancel, err := c.instantContext()
	if err != nil {
		return instant.Fulfillment{}, err
	}
	defer cancel()
	if err := c.expireInstant(ctx); err != nil {
		return instant.Fulfillment{}, fmt.Errorf("expire instant before R2 readback: %w", err)
	}
	result := instant.Fulfillment{
		DispatchID: dispatchID, RoomReady: false, NextAction: instant.ActionAwaitBooking,
	}
	var bookingID, bookingState sql.NullString
	err = c.db.QueryRowContext(ctx, `
        SELECT d.state, b.id::text, b.state
        FROM instant_dispatches d
        LEFT JOIN instant_booking_links link ON link.dispatch_id=d.id
        LEFT JOIN bookings b ON b.id=link.booking_id
        WHERE d.id=$1::uuid AND d.client_identity_id=$2::uuid`,
		dispatchID, identityID).Scan(&result.DispatchState, &bookingID, &bookingState)
	if errors.Is(err, sql.ErrNoRows) {
		return instant.Fulfillment{}, instant.ErrNotFound
	}
	if err != nil {
		return instant.Fulfillment{}, fmt.Errorf("read R6/R2 linkage: %w", err)
	}
	if bookingID.Valid && bookingState.Valid {
		result.BookingID = bookingID.String
		result.BookingState = bookingState.String
		var paid bool
		err = c.db.QueryRowContext(ctx,
			"SELECT apgic_instant_booking_payment_verified($1::uuid)",
			result.BookingID).Scan(&paid)
		if err != nil {
			return instant.Fulfillment{}, fmt.Errorf("read R2 payment evidence: %w", err)
		}
		result.PaymentVerified = paid
		if paid {
			result.NextAction = instant.ActionConsultationGate
		} else {
			result.NextAction = instant.ActionAwaitProvider
		}
	} else if result.DispatchState != instant.DispatchAccepted {
		result.NextAction = instant.ActionRetry
	}
	return result, nil
}
