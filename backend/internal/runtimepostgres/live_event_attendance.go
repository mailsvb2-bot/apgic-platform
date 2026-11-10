package runtimepostgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/liveevents"
)

// ReadLiveEventAttendance is a private aggregate for the current active
// organization owner. Only authenticated UCR media-ready snapshots prove participation; connector JOINED merely proves transport admission.
func (c *Checker) ReadLiveEventAttendance(ctx context.Context, actor, organizationID, eventID string) (liveevents.AttendanceSummary, error) {
	if c == nil || c.db == nil {
		return liveevents.AttendanceSummary{}, liveevents.ErrUnavailable
	}
	var owned bool
	err := c.db.QueryRowContext(ctx, `SELECT EXISTS(
 SELECT 1 FROM live_events e
 JOIN organization_ownerships owner ON owner.organization_id=e.organization_id
 AND owner.identity_id=$2::uuid AND owner.status='ACTIVE'
 JOIN organization_memberships member ON member.organization_id=e.organization_id
 AND member.identity_id=$2::uuid AND member.status='ACTIVE'
 JOIN organizations org ON org.id=e.organization_id AND org.status='ACTIVE'
 WHERE e.id=$1::uuid AND e.organization_id=$3::uuid
)`, eventID, actor, organizationID).Scan(&owned)
	if err != nil {
		return liveevents.AttendanceSummary{}, fmt.Errorf("check live event attendance scope: %w", err)
	}
	if !owned {
		return liveevents.AttendanceSummary{}, liveevents.ErrForbidden
	}
	var summary liveevents.AttendanceSummary
	summary.EventID = eventID
	err = c.db.QueryRowContext(ctx, `SELECT
 (SELECT count(*) FROM live_event_registrations r WHERE r.event_id=$1::uuid AND r.state='REGISTERED'),
 (SELECT count(DISTINCT identity_id) FROM live_event_attendance_snapshots
  WHERE event_id=$1::uuid),
 (SELECT count(*) FROM live_event_attendance_facts WHERE event_id=$1::uuid AND fact_type='JOINED'),
 (SELECT count(*) FROM live_event_attendance_facts WHERE event_id=$1::uuid AND fact_type='LEFT'),
 (SELECT count(*) FROM live_event_attendance_snapshots WHERE event_id=$1::uuid),
 (SELECT COALESCE(sum(connected_seconds),0) FROM (
  SELECT DISTINCT ON(identity_id) connected_seconds
  FROM live_event_attendance_snapshots WHERE event_id=$1::uuid
  ORDER BY identity_id,observed_at DESC,id DESC
 ) latest_per_person)`, eventID).
		Scan(&summary.Registered, &summary.JoinedParticipants, &summary.ProviderJoinFacts, &summary.ProviderLeaveFacts, &summary.MediaReadyObservations, &summary.TotalConnectedSeconds)
	if errors.Is(err, context.Canceled) {
		return liveevents.AttendanceSummary{}, err
	}
	if err != nil {
		return liveevents.AttendanceSummary{}, fmt.Errorf("aggregate signed attendance: %w", err)
	}
	summary.VerifiedEvidence = summary.MediaReadyObservations > 0
	return summary, nil
}
