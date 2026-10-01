package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/mobile"
)

const deepLinkReadTimeout = 3 * time.Second

func (c *Checker) DeepLinkResource(kind mobile.LinkKind, targetID string) (mobile.DeepLinkResource, bool, error) {
	if c == nil || c.db == nil {
		return mobile.DeepLinkResource{}, false, errors.New("postgres checker is not initialized")
	}
	if targetID == "" {
		return mobile.DeepLinkResource{}, false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), deepLinkReadTimeout)
	defer cancel()

	resource := mobile.DeepLinkResource{Kind: kind, TargetID: targetID}
	switch kind {
	case mobile.LinkBooking:
		if err := c.db.QueryRowContext(ctx,
			`SELECT booking.client_identity_id::text, slot.tenant_scope
			   FROM bookings booking
			   JOIN booking_slots slot ON slot.id = booking.slot_id
			  WHERE booking.id::text = $1`,
			targetID,
		).Scan(&resource.SubjectIdentityID, &resource.TenantID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return mobile.DeepLinkResource{}, false, nil
			}
			return mobile.DeepLinkResource{}, false, fmt.Errorf("read booking deep-link resource: %w", err)
		}
		resource.AccessClass = mobile.LinkProtectedResource
	case mobile.LinkNotification:
		if err := c.db.QueryRowContext(ctx,
			`SELECT booking.client_identity_id::text, slot.tenant_scope
			   FROM notification_intents intent
			   JOIN bookings booking ON booking.id = intent.booking_id
			   JOIN booking_slots slot ON slot.id = booking.slot_id
			  WHERE intent.id::text = $1`,
			targetID,
		).Scan(&resource.SubjectIdentityID, &resource.TenantID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return mobile.DeepLinkResource{}, false, nil
			}
			return mobile.DeepLinkResource{}, false, fmt.Errorf("read notification deep-link resource: %w", err)
		}
		resource.AccessClass = mobile.LinkProtectedResource
	case mobile.LinkSpecialist:
		var exists bool
		if err := c.db.QueryRowContext(ctx,
			`SELECT EXISTS (
				SELECT 1
				  FROM specialist_profiles profile
				  JOIN specialist_publications publication
				    ON publication.specialist_id = profile.id
				   AND publication.state = 'ACTIVE'
				 WHERE profile.id::text = $1
			)`,
			targetID,
		).Scan(&exists); err != nil {
			return mobile.DeepLinkResource{}, false, fmt.Errorf("read specialist deep-link resource: %w", err)
		}
		if !exists {
			return mobile.DeepLinkResource{}, false, nil
		}
		resource.AccessClass = mobile.LinkPublicResource
	default:
		return mobile.DeepLinkResource{}, false, nil
	}
	return resource, true, nil
}
