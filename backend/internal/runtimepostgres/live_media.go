package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/liveevents"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

// BindLiveMedia binds a provider-issued conference reference to APGIC's
// canonical event, scoped by an active organization owner and active connector.
// The provider room handle cannot be swapped once persisted.
func (c *Checker) LiveMediaSetupEvent(ctx context.Context, actor, eventID string) (liveevents.Event, error) {
	if c == nil || c.db == nil {
		return liveevents.Event{}, liveevents.ErrUnavailable
	}
	var owner bool
	err := c.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM live_events e
 JOIN organization_ownerships o ON o.organization_id=e.organization_id AND o.identity_id=$2::uuid AND o.status='ACTIVE'
 JOIN organization_memberships m ON m.organization_id=o.organization_id AND m.identity_id=o.identity_id AND m.status='ACTIVE'
 JOIN organizations g ON g.id=o.organization_id AND g.status='ACTIVE'
 WHERE e.id=$1::uuid)`, eventID, actor).Scan(&owner)
	if err != nil {
		return liveevents.Event{}, err
	}
	if !owner {
		return liveevents.Event{}, liveevents.ErrForbidden
	}
	return c.LiveEvent(ctx, eventID, actor)
}
func (c *Checker) BindLiveMedia(ctx context.Context, actor, eventID string, room liveevents.MediaRoom) (liveevents.MediaRoom, error) {
	if c == nil || c.db == nil {
		return liveevents.MediaRoom{}, liveevents.ErrUnavailable
	}
	if room.EventID != eventID || room.ConferenceID == "" || room.IntegrationID == "" || room.ProviderInstanceID == "" ||
		room.ProviderReadyAt.IsZero() {
		return liveevents.MediaRoom{}, liveevents.ErrInvalid
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return liveevents.MediaRoom{}, err
	}
	defer tx.Rollback()
	var state liveevents.State
	var org string
	var existingConference sql.NullString
	var existingProvider sql.NullString
	var permitted bool
	err = tx.QueryRowContext(ctx, `SELECT e.state,e.organization_id::text,e.ucr_conference_id,e.provider_instance_id::text,
 EXISTS(SELECT 1 FROM organization_ownerships o
 JOIN organization_memberships m ON m.organization_id=o.organization_id AND m.identity_id=o.identity_id AND m.status='ACTIVE'
 JOIN organizations g ON g.id=o.organization_id AND g.status='ACTIVE'
 WHERE o.organization_id=e.organization_id AND o.identity_id=$2::uuid AND o.status='ACTIVE')
 FROM live_events e WHERE e.id=$1::uuid FOR UPDATE OF e`, eventID, actor).
		Scan(&state, &org, &existingConference, &existingProvider, &permitted)
	if errors.Is(err, sql.ErrNoRows) {
		return liveevents.MediaRoom{}, liveevents.ErrNotFound
	}
	if err != nil {
		return liveevents.MediaRoom{}, err
	}
	if !permitted || org != room.OrganizationID {
		return liveevents.MediaRoom{}, liveevents.ErrForbidden
	}
	if state != liveevents.Draft && state != liveevents.Published && state != liveevents.Live {
		return liveevents.MediaRoom{}, liveevents.ErrConflict
	}
	if existingConference.Valid && (existingConference.String != room.ConferenceID || !existingProvider.Valid || existingProvider.String != room.ProviderInstanceID) {
		return liveevents.MediaRoom{}, liveevents.ErrConflict
	}
	var active bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM connector_instances
 WHERE id=$1::uuid AND capability_class='COMMUNICATION_PROVIDER' AND status='ACTIVE')`,
		room.ProviderInstanceID).Scan(&active); err != nil {
		return liveevents.MediaRoom{}, err
	}
	if !active {
		return liveevents.MediaRoom{}, liveevents.ErrUnavailable
	}
	if _, err = tx.ExecContext(ctx, `UPDATE live_events SET provider_instance_id=$2::uuid,
 provider_proof_checked_at=$3,ucr_conference_id=$4,ucr_integration_id=$5,updated_at=clock_timestamp()
 WHERE id=$1::uuid`, eventID, room.ProviderInstanceID, room.ProviderReadyAt,
		room.ConferenceID, room.IntegrationID); err != nil {
		return liveevents.MediaRoom{}, fmt.Errorf("bind verified UCR conference: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return liveevents.MediaRoom{}, err
	}
	room.State = state
	return room, nil
}
func (c *Checker) LiveMediaForOwner(ctx context.Context, actor, eventID string) (liveevents.MediaRoom, error) {
	if c == nil || c.db == nil {
		return liveevents.MediaRoom{}, liveevents.ErrUnavailable
	}
	var out liveevents.MediaRoom
	var provider, conference, integration sql.NullString
	err := c.db.QueryRowContext(ctx, `SELECT e.id::text,e.organization_id::text,e.state,e.starts_at,e.ends_at,
 e.capacity,e.format,e.provider_instance_id::text,e.ucr_conference_id,e.ucr_integration_id,
 e.provider_proof_checked_at
 FROM live_events e
 JOIN organization_ownerships o ON o.organization_id=e.organization_id
 JOIN organization_memberships m ON m.organization_id=e.organization_id AND m.identity_id=o.identity_id
 JOIN organizations g ON g.id=e.organization_id
 WHERE e.id=$1::uuid AND o.identity_id=$2::uuid AND o.status='ACTIVE' AND m.status='ACTIVE'
 AND g.status='ACTIVE'`, eventID, actor).
		Scan(&out.EventID, &out.OrganizationID, &out.State, &out.StartsAt, &out.EndsAt,
			&out.Capacity, &out.Format, &provider, &conference, &integration, &out.ProviderReadyAt)
	if errors.Is(err, sql.ErrNoRows) {
		return liveevents.MediaRoom{}, liveevents.ErrForbidden
	}
	if err != nil {
		return liveevents.MediaRoom{}, err
	}
	if !conference.Valid || !provider.Valid || !integration.Valid {
		return liveevents.MediaRoom{}, liveevents.ErrUnavailable
	}
	out.ProviderInstanceID = provider.String
	out.ConferenceID = conference.String
	out.IntegrationID = integration.String
	return out, nil
}
func (c *Checker) LiveMediaForJoin(ctx context.Context, actor, eventID string, now time.Time) (liveevents.MediaRoom, string, error) {
	if c == nil || c.db == nil {
		return liveevents.MediaRoom{}, "", liveevents.ErrUnavailable
	}
	var out liveevents.MediaRoom
	var provider, conference, integration sql.NullString
	var owner, registered bool
	var providerActive bool
	err := c.db.QueryRowContext(ctx, `SELECT e.id::text,e.organization_id::text,e.state,e.starts_at,e.ends_at,
 e.capacity,e.format,e.provider_instance_id::text,e.ucr_conference_id,e.ucr_integration_id,
 e.provider_proof_checked_at,
 EXISTS(SELECT 1 FROM organization_ownerships o
 JOIN organization_memberships m ON m.organization_id=o.organization_id AND m.identity_id=o.identity_id AND m.status='ACTIVE'
 WHERE o.organization_id=e.organization_id AND o.identity_id=$2::uuid AND o.status='ACTIVE'),
 EXISTS(SELECT 1 FROM live_event_registrations r WHERE r.event_id=e.id
 AND r.identity_id=$2::uuid AND r.state='REGISTERED'
 AND NOT EXISTS(SELECT 1 FROM live_event_bans b WHERE b.event_id=e.id AND b.identity_id=r.identity_id)),
 EXISTS(SELECT 1 FROM connector_instances p WHERE p.id=e.provider_instance_id AND p.status='ACTIVE'
 AND p.capability_class='COMMUNICATION_PROVIDER')
 FROM live_events e JOIN organizations org ON org.id=e.organization_id AND org.status='ACTIVE'
 WHERE e.id=$1::uuid`, eventID, actor).
		Scan(&out.EventID, &out.OrganizationID, &out.State, &out.StartsAt, &out.EndsAt,
			&out.Capacity, &out.Format, &provider, &conference, &integration, &out.ProviderReadyAt, &owner, &registered, &providerActive)
	if errors.Is(err, sql.ErrNoRows) {
		return liveevents.MediaRoom{}, "", liveevents.ErrNotFound
	}
	if err != nil {
		return liveevents.MediaRoom{}, "", err
	}
	if !owner && !registered {
		return liveevents.MediaRoom{}, "", liveevents.ErrForbidden
	}
	if out.State != liveevents.Published && out.State != liveevents.Live {
		return liveevents.MediaRoom{}, "", liveevents.ErrConflict
	}
	if now.Before(out.StartsAt.Add(-15*time.Minute)) || !now.Before(out.EndsAt.Add(15*time.Minute)) {
		return liveevents.MediaRoom{}, "", liveevents.ErrConflict
	}
	if !provider.Valid || !conference.Valid || !integration.Valid || !providerActive {
		return liveevents.MediaRoom{}, "", liveevents.ErrUnavailable
	}
	out.ProviderInstanceID = provider.String
	out.ConferenceID = conference.String
	out.IntegrationID = integration.String
	if owner {
		return out, "host", nil
	}
	return out, "attendee", nil
}
func (c *Checker) LiveMediaRegistered(ctx context.Context, actor, eventID string) ([]string, error) {
	if c == nil || c.db == nil {
		return nil, liveevents.ErrUnavailable
	}
	if _, err := c.LiveMediaForOwner(ctx, actor, eventID); err != nil {
		return nil, err
	}
	rows, err := c.db.QueryContext(ctx, `SELECT identity_id::text FROM live_event_registrations
 WHERE event_id=$1::uuid AND state='REGISTERED' ORDER BY identity_id`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (c *Checker) BanLiveParticipant(ctx context.Context, actor, eventID, identity string, at time.Time) error {
	if c == nil || c.db == nil {
		return liveevents.ErrUnavailable
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var org string
	var permitted bool
	err = tx.QueryRowContext(ctx, `SELECT e.organization_id::text,
 EXISTS(SELECT 1 FROM organization_ownerships o
 JOIN organization_memberships m ON m.organization_id=o.organization_id AND m.identity_id=o.identity_id AND m.status='ACTIVE'
 WHERE o.organization_id=e.organization_id AND o.identity_id=$2::uuid AND o.status='ACTIVE')
 FROM live_events e WHERE e.id=$1::uuid AND e.state IN ('PUBLISHED','LIVE') FOR UPDATE OF e`, eventID, actor).
		Scan(&org, &permitted)
	if errors.Is(err, sql.ErrNoRows) {
		return liveevents.ErrNotFound
	}
	if err != nil {
		return err
	}
	if !permitted {
		return liveevents.ErrForbidden
	}
	var registered bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM live_event_registrations
 WHERE event_id=$1::uuid AND identity_id=$2::uuid AND state='REGISTERED')`, eventID, identity).Scan(&registered)
	if err != nil {
		return err
	}
	if !registered {
		return liveevents.ErrNotFound
	}
	var added int64
	res, err := tx.ExecContext(ctx, `INSERT INTO live_event_bans(event_id,identity_id,moderator_identity_id,banned_at)
 VALUES($1::uuid,$2::uuid,$3::uuid,$4)
 ON CONFLICT(event_id,identity_id) DO NOTHING`, eventID, identity, actor, at)
	if err != nil {
		return err
	}
	added, err = res.RowsAffected()
	if err != nil {
		return err
	}
	if added > 0 {
		if err = appendLiveEventOutbox(ctx, tx, eventID, org, actor, "live_event.participant_removed", at); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (c *Checker) SaveLiveAttendance(ctx context.Context, room liveevents.MediaRoom, identity string, fact liveevents.MediaParticipantFact, at time.Time) error {
	if c == nil || c.db == nil {
		return liveevents.ErrUnavailable
	}
	if !fact.Joined {
		return nil
	}
	if fact.JoinCount < 1 || fact.ConnectedSeconds < 0 || fact.FirstJoinedAt.IsZero() {
		return liveevents.ErrInvalid
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	id, err := persistentid.New()
	if err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO live_event_attendance_snapshots(id,event_id,identity_id,
 provider_instance_id,ucr_conference_id,join_count,connected_seconds,first_join_at,observed_at)
 VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6,$7,$8,$9)
 ON CONFLICT(event_id,identity_id,join_count,connected_seconds) DO NOTHING`,
		id, room.EventID, identity, room.ProviderInstanceID, room.ConferenceID, fact.JoinCount,
		fact.ConnectedSeconds, fact.FirstJoinedAt, at)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 1 {
		if err = appendLiveEventOutbox(ctx, tx, room.EventID, room.OrganizationID, identity, "live_event.attendance_verified", at); err != nil {
			return err
		}
	}
	return tx.Commit()
}
