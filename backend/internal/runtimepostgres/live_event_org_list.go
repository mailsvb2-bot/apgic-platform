package runtimepostgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/liveevents"
)

// ListOrganizationLiveEvents is intentionally owner-only, including DRAFT and
// CANCELLED records. No caller-supplied organization ID bypasses membership.
func (c *Checker) ListOrganizationLiveEvents(ctx context.Context, actor, organizationID string) ([]liveevents.Event, error) {
	if c == nil || c.db == nil {
		return nil, liveevents.ErrUnavailable
	}
	var allowed bool
	if err := c.db.QueryRowContext(ctx, `SELECT EXISTS(
SELECT 1 FROM organization_ownerships ownership
JOIN organization_memberships membership
 ON membership.organization_id = ownership.organization_id
 AND membership.identity_id = ownership.identity_id
JOIN organizations organization ON organization.id = ownership.organization_id
WHERE ownership.organization_id = $1::uuid
 AND ownership.identity_id = $2::uuid
 AND ownership.status = 'ACTIVE'
 AND membership.status = 'ACTIVE'
 AND organization.status = 'ACTIVE'
)`, organizationID, actor).Scan(&allowed); err != nil {
		return nil, fmt.Errorf("verify live event organizer: %w", err)
	}
	if !allowed {
		return nil, liveevents.ErrForbidden
	}
	rows, err := c.db.QueryContext(ctx, `SELECT id::text FROM live_events
WHERE organization_id=$1::uuid ORDER BY starts_at DESC, id LIMIT 250`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list organization live events: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := make([]liveevents.Event, 0, len(ids))
	for _, id := range ids {
		item, err := liveEventByID(ctx, c.db, id, actor)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, liveevents.ErrNotFound
			}
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}
