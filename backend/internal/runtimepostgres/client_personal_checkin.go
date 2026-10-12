package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/clientpersonal"
)

// The user cannot attest provider completion; SQL independently checks the provider-owned lifecycle.
func (c *Checker) CreateClientCheckIn(ctx context.Context, identity string, v clientpersonal.CheckInInput) (clientpersonal.CheckIn, error) {
	var empty clientpersonal.CheckIn
	if c == nil || c.db == nil {
		return empty, errors.New("postgres unavailable")
	}
	if err := clientpersonal.ValidateCheckIn(v); err != nil {
		return empty, err
	}
	ctx, cancel := context.WithTimeout(ctx, clientPersonalTimeout)
	defer cancel()
	var at time.Time
	err := c.db.QueryRowContext(ctx, `INSERT INTO client_personal_check_ins
		(booking_id,client_identity_id,progress_code,match_satisfaction)
		SELECT b.id,b.client_identity_id,$3,$4 FROM bookings b
		JOIN consultation_sessions cs ON cs.booking_id=b.id
		WHERE b.id=$1::uuid AND b.client_identity_id=$2::uuid
		AND cs.client_identity_id=b.client_identity_id AND cs.state='COMPLETED'
		ON CONFLICT (booking_id) DO NOTHING RETURNING recorded_at`,
		v.BookingID, identity, v.ProgressCode, v.MatchSatisfaction).Scan(&at)
	if err == nil {
		return clientpersonal.CheckIn{CheckInInput: v, RecordedAt: at}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return empty, fmt.Errorf("insert client report: %w", err)
	}
	var progress string
	var satisfaction sql.NullInt16
	err = c.db.QueryRowContext(ctx, `SELECT progress_code,match_satisfaction,recorded_at
		FROM client_personal_check_ins WHERE booking_id=$1::uuid AND client_identity_id=$2::uuid`,
		v.BookingID, identity).Scan(&progress, &satisfaction, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return empty, clientpersonal.ErrCheckInNotEligible
	}
	if err != nil {
		return empty, fmt.Errorf("read client report: %w", err)
	}
	if progress != v.ProgressCode || satisfaction.Valid != (v.MatchSatisfaction != nil) ||
		(satisfaction.Valid && int(satisfaction.Int16) != *v.MatchSatisfaction) {
		return empty, clientpersonal.ErrCheckInConflict
	}
	return clientpersonal.CheckIn{CheckInInput: v, RecordedAt: at, Idempotent: true}, nil
}
