package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/instant"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

const instantTimeout = 5 * time.Second
const zeroUUID = "00000000-0000-0000-0000-000000000000"

func (c *Checker) instantContext() (context.Context, context.CancelFunc, error) {
	if c == nil || c.db == nil {
		return nil, nil, instant.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), instantTimeout)
	return ctx, cancel, nil
}

func (c *Checker) SetPresence(identityID, offerID string, available bool) (instant.Presence, error) {
	if !instant.ValidID(identityID) || (available && !instant.ValidID(offerID)) {
		return instant.Presence{}, instant.ErrInvalid
	}
	ctx, cancel, err := c.instantContext()
	if err != nil {
		return instant.Presence{}, err
	}
	defer cancel()
	if !available {
		offerID = zeroUUID
	}
	var reason string
	err = c.db.QueryRowContext(ctx, "SELECT apgic_instant_enable($1::uuid,$2::uuid,$3,$4)", identityID, offerID, available, time.Now().UTC()).Scan(&reason)
	if err != nil {
		return instant.Presence{}, fmt.Errorf("set duty presence: %w", err)
	}
	switch reason {
	case "DUTY_BUSY", "DUTY_SCHEDULE_CONFLICT":
		return instant.Presence{}, instant.ErrConflict
	case "DUTY_NOT_ELIGIBLE":
		return instant.Presence{}, instant.ErrNotEligible
	case "DUTY_OFFLINE", "DUTY_AVAILABLE_NOW":
		return c.GetPresence(identityID)
	default:
		return instant.Presence{}, fmt.Errorf("unknown duty presence reason %q", reason)
	}
}

func (c *Checker) expireInstant(ctx context.Context) error {
	_, err := c.db.ExecContext(ctx, "SELECT apgic_instant_expire($1)", time.Now().UTC())
	return err
}

func (c *Checker) GetPresence(identityID string) (instant.Presence, error) {
	if !instant.ValidID(identityID) {
		return instant.Presence{}, instant.ErrInvalid
	}
	ctx, cancel, err := c.instantContext()
	if err != nil {
		return instant.Presence{}, err
	}
	defer cancel()
	if err := c.expireInstant(ctx); err != nil {
		return instant.Presence{}, fmt.Errorf("expire duty presence: %w", err)
	}
	var result instant.Presence
	var until sql.NullTime
	err = c.db.QueryRowContext(ctx, `SELECT specialist_identity_id::text,offer_id::text,state,available_until,version
   FROM instant_presence WHERE specialist_identity_id=$1::uuid`, identityID).
		Scan(&result.SpecialistIdentityID, &result.OfferID, &result.State, &until, &result.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return instant.Presence{SpecialistIdentityID: identityID, State: instant.StateOffline}, nil
	}
	if err != nil {
		return instant.Presence{}, err
	}
	if until.Valid {
		result.AvailableUntil = &until.Time
	}
	return result, nil
}

func (c *Checker) Request(identityID, intentID, requestKey string) (instant.Dispatch, error) {
	if !instant.ValidID(identityID) || !instant.ValidID(intentID) || !instant.ValidID(requestKey) {
		return instant.Dispatch{}, instant.ErrInvalid
	}
	ctx, cancel, err := c.instantContext()
	if err != nil {
		return instant.Dispatch{}, err
	}
	defer cancel()
	id, err := persistentid.New()
	if err != nil {
		return instant.Dispatch{}, err
	}
	var reason string
	err = c.db.QueryRowContext(ctx, `SELECT apgic_instant_request($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5)`,
		id, requestKey, identityID, intentID, time.Now().UTC()).Scan(&reason)
	if err != nil {
		return instant.Dispatch{}, fmt.Errorf("duty dispatch: %w", err)
	}
	switch reason {
	case "DUTY_INTENT_NOT_ELIGIBLE":
		return instant.Dispatch{}, instant.ErrNotEligible
	case "DUTY_ALREADY_ACTIVE", "DUTY_IDEMPOTENCY_CONFLICT":
		return instant.Dispatch{}, instant.ErrConflict
	case "DUTY_OFFERED", "DUTY_NO_SUPPLY", "DUTY_IDEMPOTENT_REPLAY":
		return c.dispatchByKey(ctx, identityID, requestKey)
	default:
		return instant.Dispatch{}, fmt.Errorf("unknown duty dispatch reason %q", reason)
	}
}

func (c *Checker) dispatchByKey(ctx context.Context, identityID, requestKey string) (instant.Dispatch, error) {
	return c.scanInstant(c.db.QueryRowContext(ctx, instantSelect+` WHERE client_identity_id=$1::uuid AND request_key=$2::uuid`, identityID, requestKey))
}

const instantSelect = `SELECT id::text,request_key::text,client_identity_id::text,help_intent_id::text,
 COALESCE(specialist_identity_id::text,''),COALESCE(offer_id::text,''),COALESCE(topic_id,''),
 COALESCE(price_minor,0),COALESCE(currency,''),state,offered_at,expires_at,responded_at
 FROM instant_dispatches`

type instantScanner interface{ Scan(...any) error }

func (c *Checker) scanInstant(row instantScanner) (instant.Dispatch, error) {
	var d instant.Dispatch
	var offered, expires, responded sql.NullTime
	err := row.Scan(&d.ID, &d.RequestKey, &d.ClientIdentityID, &d.HelpIntentID,
		&d.SpecialistIdentityID, &d.OfferID, &d.TopicID, &d.PriceMinor, &d.Currency,
		&d.State, &offered, &expires, &responded)
	if errors.Is(err, sql.ErrNoRows) {
		return instant.Dispatch{}, instant.ErrNotFound
	}
	if err != nil {
		return instant.Dispatch{}, err
	}
	if offered.Valid {
		d.OfferedAt = &offered.Time
	}
	if expires.Valid {
		d.ExpiresAt = &expires.Time
	}
	if responded.Valid {
		d.RespondedAt = &responded.Time
	}
	d.Normalize()
	return d, nil
}

func (c *Checker) GetDispatch(id, identityID string) (instant.Dispatch, error) {
	if !instant.ValidID(id) || !instant.ValidID(identityID) {
		return instant.Dispatch{}, instant.ErrInvalid
	}
	ctx, cancel, err := c.instantContext()
	if err != nil {
		return instant.Dispatch{}, err
	}
	defer cancel()
	if err := c.expireInstant(ctx); err != nil {
		return instant.Dispatch{}, fmt.Errorf("expire duty dispatch: %w", err)
	}
	return c.scanInstant(c.db.QueryRowContext(ctx,
		instantSelect+` WHERE id=$1::uuid AND (client_identity_id=$2::uuid OR specialist_identity_id=$2::uuid)`,
		id, identityID))
}

// ActiveDispatch is the authoritative reconnect snapshot. It never infers
// room authorization from a specialist's agreement to take a request.
func (c *Checker) ActiveDispatch(identityID string) (instant.Dispatch, error) {
	if !instant.ValidID(identityID) {
		return instant.Dispatch{}, instant.ErrInvalid
	}
	ctx, cancel, err := c.instantContext()
	if err != nil {
		return instant.Dispatch{}, err
	}
	defer cancel()
	if err := c.expireInstant(ctx); err != nil {
		return instant.Dispatch{}, fmt.Errorf("expire active duty dispatch: %w", err)
	}
	return c.scanInstant(c.db.QueryRowContext(ctx,
		instantSelect+` WHERE client_identity_id=$1::uuid AND state IN ('OFFERED','ACCEPTED') ORDER BY created_at DESC LIMIT 1`,
		identityID))
}

func (c *Checker) Answer(id, specialistID string, accept bool) (instant.Dispatch, error) {
	if !instant.ValidID(id) || !instant.ValidID(specialistID) {
		return instant.Dispatch{}, instant.ErrInvalid
	}
	ctx, cancel, err := c.instantContext()
	if err != nil {
		return instant.Dispatch{}, err
	}
	defer cancel()
	var reason string
	err = c.db.QueryRowContext(ctx, `SELECT apgic_instant_answer($1::uuid,$2::uuid,$3,$4)`,
		id, specialistID, accept, time.Now().UTC()).Scan(&reason)
	if err != nil {
		return instant.Dispatch{}, fmt.Errorf("answer duty offer: %w", err)
	}
	switch reason {
	case "DUTY_NOT_FOUND":
		return instant.Dispatch{}, instant.ErrNotFound
	case "DUTY_OFFER_NOT_ACTIVE", "DUTY_SCHEDULE_CONFLICT":
		return instant.Dispatch{}, instant.ErrConflict
	case "DUTY_NOT_ELIGIBLE":
		return instant.Dispatch{}, instant.ErrNotEligible
	case "DUTY_ACCEPTED_PAYMENT_REQUIRED", "DUTY_ALREADY_ACCEPTED", "DUTY_DECLINED":
		return c.GetDispatch(id, specialistID)
	default:
		return instant.Dispatch{}, fmt.Errorf("unknown duty answer reason %q", reason)
	}
}

// Cancel is owned exclusively by the signed client. It persists a terminal
// state and releases the matching specialist; it never cancels money or room
// access, neither of which this R6 prepayment subsystem can grant.
func (c *Checker) Cancel(id, clientIdentityID string) (instant.Dispatch, error) {
	if !instant.ValidID(id) || !instant.ValidID(clientIdentityID) {
		return instant.Dispatch{}, instant.ErrInvalid
	}
	ctx, cancel, err := c.instantContext()
	if err != nil {
		return instant.Dispatch{}, err
	}
	defer cancel()
	var reason string
	err = c.db.QueryRowContext(ctx, `SELECT apgic_instant_cancel($1::uuid,$2::uuid,$3)`,
		id, clientIdentityID, time.Now().UTC()).Scan(&reason)
	if err != nil {
		return instant.Dispatch{}, fmt.Errorf("cancel duty dispatch: %w", err)
	}
	switch reason {
	case "DUTY_NOT_FOUND":
		return instant.Dispatch{}, instant.ErrNotFound
	case "DUTY_NOT_CANCELLABLE":
		return instant.Dispatch{}, instant.ErrConflict
	case "DUTY_CANCELLED", "DUTY_ALREADY_CANCELLED":
		return c.GetDispatch(id, clientIdentityID)
	default:
		return instant.Dispatch{}, fmt.Errorf("unknown duty cancellation reason %q", reason)
	}
}

// Polling returns only the authenticated specialist's inbox, never another
// tenant's HelpIntent text or unrelated client-sensitive records.
func (c *Checker) Inbox(specialistID string) ([]instant.Dispatch, error) {
	if !instant.ValidID(specialistID) {
		return nil, instant.ErrInvalid
	}
	ctx, cancel, err := c.instantContext()
	if err != nil {
		return nil, err
	}
	defer cancel()
	if err := c.expireInstant(ctx); err != nil {
		return nil, fmt.Errorf("expire duty inbox: %w", err)
	}
	rows, err := c.db.QueryContext(ctx, instantSelect+`
  WHERE specialist_identity_id=$1::uuid AND state IN ('OFFERED','ACCEPTED')
  ORDER BY created_at DESC LIMIT 20`, specialistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]instant.Dispatch, 0)
	for rows.Next() {
		d, err := c.scanInstant(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}
