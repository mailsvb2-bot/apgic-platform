package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/clientpersonal"
)

const clientPersonalTimeout = 5 * time.Second

// ReadClientPersonal reads canonical booking/consultation/order/intent truth.
// Neither a browser nor a device cache may manufacture history or completion.
func (c *Checker) ReadClientPersonal(ctx context.Context, identityID string, limit int, cursor *clientpersonal.HistoryCursor, now time.Time) (clientpersonal.View, error) {
	var view clientpersonal.View
	if c == nil || c.db == nil {
		return view, errors.New("postgres unavailable")
	}
	if limit < 1 || limit > clientpersonal.MaxHistoryLimit {
		return view, clientpersonal.ErrInvalidCursor
	}
	ctx, cancel := context.WithTimeout(ctx, clientPersonalTimeout)
	defer cancel()
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return view, fmt.Errorf("begin client personal snapshot: %w", err)
	}
	defer tx.Rollback()
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM identities WHERE id=$1::uuid)`, identityID).Scan(&exists); err != nil {
		return view, fmt.Errorf("identity lookup: %w", err)
	}
	if !exists {
		return view, clientpersonal.ErrIdentityNotFound
	}
	view = clientpersonal.View{IdentityID: identityID, AsOf: now.UTC(), Preferences: clientpersonal.DefaultPreferences(), Upcoming: []clientpersonal.Booking{}, History: []clientpersonal.Booking{}, Requests: []clientpersonal.Request{}}
	var updated sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT display_name,locale,time_zone,updated_at FROM client_personal_preferences WHERE identity_id=$1::uuid`, identityID).
		Scan(&view.Preferences.DisplayName, &view.Preferences.Locale, &view.Preferences.TimeZone, &updated)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return view, fmt.Errorf("read personal preferences: %w", err)
	}
	if updated.Valid {
		view.Preferences.UpdatedAt = &updated.Time
	}
	const bookingColumns = `SELECT b.id::text,b.state,b.starts_at,b.ends_at,
        COALESCE(sp.display_name,'Специалист'),COALESCE(sp.profession_code,''),
        cs.state,o.id::text,ci.progress_code,ci.match_satisfaction
        FROM bookings b
        JOIN booking_slots bs ON bs.id=b.slot_id
        LEFT JOIN specialist_profiles sp ON sp.identity_id=bs.specialist_identity_id
        LEFT JOIN consultation_sessions cs ON cs.booking_id=b.id AND cs.client_identity_id=b.client_identity_id
        LEFT JOIN orders o ON o.booking_id=b.id
        LEFT JOIN client_personal_check_ins ci ON ci.booking_id=b.id AND ci.client_identity_id=b.client_identity_id
        WHERE b.client_identity_id=$1::uuid`
	upcoming, err := tx.QueryContext(ctx, bookingColumns+` AND b.starts_at >= $2 AND b.state IN ('HELD','PENDING_PAYMENT','CONFIRMED')
        ORDER BY b.starts_at ASC,b.id ASC LIMIT 30`, identityID, now.UTC())
	if err != nil {
		return view, fmt.Errorf("read upcoming bookings: %w", err)
	}
	view.Upcoming, err = scanPersonalBookings(upcoming)
	if err != nil {
		return view, err
	}
	beforeAt := time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	beforeID := "ffffffff-ffff-ffff-ffff-ffffffffffff"
	if cursor != nil {
		beforeAt = cursor.StartsAt
		beforeID = cursor.BookingID
	}
	history, err := tx.QueryContext(ctx, bookingColumns+` AND (b.starts_at < $2 OR b.state IN ('CANCELLED','EXPIRED','COMPLETED','NO_SHOW'))
        AND (b.starts_at,b.id)<($3::timestamptz,$4::uuid)
        ORDER BY b.starts_at DESC,b.id DESC LIMIT $5`, identityID, now.UTC(), beforeAt, beforeID, limit+1)
	if err != nil {
		return view, fmt.Errorf("read client booking history: %w", err)
	}
	view.History, err = scanPersonalBookings(history)
	if err != nil {
		return view, err
	}
	if len(view.History) > limit {
		view.History = view.History[:limit]
		last := view.History[len(view.History)-1]
		view.NextHistoryCursor = clientpersonal.EncodeCursor(last.StartsAt, last.ID)
	}
	// No raw free_text or consultation transcript in the personal summary.
	requests, err := tx.QueryContext(ctx, `SELECT id::text,state,topics,created_at
        FROM help_intents WHERE identity_id=$1::uuid ORDER BY created_at DESC,id DESC LIMIT 10`, identityID)
	if err != nil {
		return view, fmt.Errorf("read client requests: %w", err)
	}
	defer requests.Close()
	for requests.Next() {
		var item clientpersonal.Request
		if err = requests.Scan(&item.ID, &item.State, &item.Topics, &item.CreatedAt); err != nil {
			return view, fmt.Errorf("scan client request: %w", err)
		}
		if item.Topics == nil {
			item.Topics = []string{}
		}
		view.Requests = append(view.Requests, item)
	}
	if err = requests.Err(); err != nil {
		return view, fmt.Errorf("iterate client requests: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return view, fmt.Errorf("commit client personal snapshot: %w", err)
	}
	return view, nil
}

func scanPersonalBookings(rows *sql.Rows) ([]clientpersonal.Booking, error) {
	defer rows.Close()
	out := []clientpersonal.Booking{}
	for rows.Next() {
		var b clientpersonal.Booking
		var consultationState, orderID sql.NullString
		var progress sql.NullString
		var satisfaction sql.NullInt16
		if err := rows.Scan(&b.ID, &b.State, &b.StartsAt, &b.EndsAt, &b.SpecialistName, &b.SpecialistProfession, &consultationState, &orderID, &progress, &satisfaction); err != nil {
			return nil, fmt.Errorf("scan client booking: %w", err)
		}
		if consultationState.Valid {
			b.ConsultationState = &consultationState.String
		}
		if orderID.Valid {
			b.OrderID = &orderID.String
		}
		if progress.Valid {
			b.ClientProgress = &progress.String
		}
		if satisfaction.Valid {
			n := int(satisfaction.Int16)
			b.MatchSatisfaction = &n
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate client bookings: %w", err)
	}
	return out, nil
}

func (c *Checker) UpdateClientPersonalPreferences(ctx context.Context, identityID string, p clientpersonal.Preferences) (clientpersonal.Preferences, error) {
	if c == nil || c.db == nil {
		return p, errors.New("postgres unavailable")
	}
	if err := clientpersonal.ValidatePreferences(p); err != nil {
		return p, err
	}
	ctx, cancel := context.WithTimeout(ctx, clientPersonalTimeout)
	defer cancel()
	p.DisplayName = strings.TrimSpace(p.DisplayName)
	var updated time.Time
	err := c.db.QueryRowContext(ctx, `INSERT INTO client_personal_preferences(identity_id,display_name,locale,time_zone)
        VALUES($1::uuid,$2,$3,$4)
        ON CONFLICT(identity_id) DO UPDATE SET display_name=EXCLUDED.display_name,
        locale=EXCLUDED.locale,time_zone=EXCLUDED.time_zone,updated_at=now()
        RETURNING updated_at`, identityID, p.DisplayName, p.Locale, p.TimeZone).Scan(&updated)
	if err != nil {
		return p, fmt.Errorf("save personal preferences: %w", err)
	}
	p.UpdatedAt = &updated
	return p, nil
}
