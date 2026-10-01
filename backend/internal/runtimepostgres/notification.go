package runtimepostgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/notification"
)

func (c *Checker) ClaimNotificationDelivery(ctx context.Context, plan notification.DeliveryPlan, decision notification.DeliveryAudit) (notification.DeliveryPlan, bool, error) {
	if c == nil || c.db == nil {
		return notification.DeliveryPlan{}, false, errors.New("postgres checker is not initialized")
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return notification.DeliveryPlan{}, false, err
	}
	defer tx.Rollback()

	var bookingID string
	var dataClass string
	var policyVersion string
	var channelAllowed bool
	var sensitivePreviewPolicy string
	err = tx.QueryRowContext(ctx, `
		SELECT booking_id::text,
		       data_class,
		       channel_policy_version,
		       COALESCE((channel_preferences ->> $2)::boolean, false),
		       sensitive_preview_policy
		FROM notification_intents
		WHERE id::text = $1
	`, plan.IntentID, string(plan.Channel)).Scan(
		&bookingID,
		&dataClass,
		&policyVersion,
		&channelAllowed,
		&sensitivePreviewPolicy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return notification.DeliveryPlan{}, false, notification.ErrInvalidDelivery
	}
	if err != nil {
		return notification.DeliveryPlan{}, false, err
	}
	if bookingID != plan.BookingID || dataClass != plan.DataClass || policyVersion != plan.PolicyVersion {
		return notification.DeliveryPlan{}, false, notification.ErrInvalidDelivery
	}

	if !channelAllowed {
		plan.State = notification.DeliverySuppressed
		plan.ReasonCode = notification.ReasonDeliveryPreferenceSuppressed
	}
	plan.PreviewMode = notification.PreviewModeFor(
		dataClass,
		plan.Channel,
		strings.EqualFold(sensitivePreviewPolicy, "FULL_ALLOWED"),
	)

	if plan.State == notification.DeliverySuppressed {
		var duplicate bool
		if err := tx.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM audit_records
				WHERE correlation_id = $1
				  AND action = 'NOTIFICATION_DELIVERY_DECISION'
			)
		`, plan.IdempotencyKey).Scan(&duplicate); err != nil {
			return notification.DeliveryPlan{}, false, err
		}
		if duplicate {
			plan.ReasonCode = notification.ReasonDeliveryDuplicate
		}
		if err := appendNotificationDeliveryAudit(ctx, tx, plan, decision, duplicate); err != nil {
			return notification.DeliveryPlan{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return notification.DeliveryPlan{}, false, err
		}
		return plan, duplicate, nil
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO notification_deliveries (
			id, intent_id, channel, endpoint_ref, provider_instance_id,
			delivery_idempotency_key, state, created_at, updated_at
		) VALUES ($1::uuid, $2::uuid, $3, $4, $5::uuid, $6, 'PENDING', $7, $7)
		ON CONFLICT (delivery_idempotency_key) DO NOTHING
	`,
		plan.DeliveryID,
		plan.IntentID,
		string(plan.Channel),
		plan.EndpointRef,
		plan.ProviderInstanceID,
		plan.IdempotencyKey,
		plan.OccurredAt,
	)
	if err != nil {
		return notification.DeliveryPlan{}, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return notification.DeliveryPlan{}, false, err
	}
	duplicate := rows == 0
	if duplicate {
		var existing notification.DeliveryPlan
		var channel string
		var state string
		err := tx.QueryRowContext(ctx, `
			SELECT id::text, intent_id::text, channel, endpoint_ref,
			       provider_instance_id::text, delivery_idempotency_key, state, created_at
			FROM notification_deliveries
			WHERE delivery_idempotency_key = $1
		`, plan.IdempotencyKey).Scan(
			&existing.DeliveryID,
			&existing.IntentID,
			&channel,
			&existing.EndpointRef,
			&existing.ProviderInstanceID,
			&existing.IdempotencyKey,
			&state,
			&existing.OccurredAt,
		)
		if err != nil {
			return notification.DeliveryPlan{}, false, err
		}
		existing.Channel = notification.Channel(channel)
		existing.State = notification.DeliveryState(state)
		existing.BookingID = plan.BookingID
		existing.DataClass = plan.DataClass
		existing.PolicyVersion = plan.PolicyVersion
		existing.PreviewMode = plan.PreviewMode
		existing.ReasonCode = notification.ReasonDeliveryDuplicate
		if existing.IntentID != plan.IntentID || existing.Channel != plan.Channel ||
			existing.EndpointRef != plan.EndpointRef || existing.ProviderInstanceID != plan.ProviderInstanceID {
			return notification.DeliveryPlan{}, false, notification.ErrDeliveryIdempotencyConflict
		}
		plan = existing
	}
	if err := appendNotificationDeliveryAudit(ctx, tx, plan, decision, duplicate); err != nil {
		return notification.DeliveryPlan{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return notification.DeliveryPlan{}, false, err
	}
	return plan, duplicate, nil
}

func appendNotificationDeliveryAudit(ctx context.Context, tx *sql.Tx, plan notification.DeliveryPlan, decision notification.DeliveryAudit, duplicate bool) error {
	reason := plan.ReasonCode
	if duplicate {
		reason = notification.ReasonDeliveryDuplicate
	}
	state, err := json.Marshal(map[string]any{
		"intent_id":       plan.IntentID,
		"channel":         plan.Channel,
		"delivery_state":  plan.State,
		"preview_mode":    plan.PreviewMode,
		"idempotency_key": plan.IdempotencyKey,
		"duplicate":       duplicate,
	})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO audit_records (
			id, actor_id, action, scope, resource_ref, new_state,
			reason, policy_version, correlation_id, occurred_at
		) VALUES ($1::uuid, $2, 'NOTIFICATION_DELIVERY_DECISION', 'notification.delivery', $3,
		          $4::jsonb, $5, $6, $7, $8)
	`,
		decision.ID,
		decision.ActorID,
		"notification-delivery/"+plan.DeliveryID,
		string(state),
		reason,
		plan.PolicyVersion,
		plan.IdempotencyKey,
		decision.OccurredAt,
	)
	return err
}

func (c *Checker) MobileNotificationDelivery(ctx context.Context, identityID, deliveryID string) (notification.MobileDeliveryProjection, bool, error) {
	if c == nil || c.db == nil {
		return notification.MobileDeliveryProjection{}, false, errors.New("postgres checker is not initialized")
	}
	if strings.TrimSpace(identityID) == "" || strings.TrimSpace(deliveryID) == "" {
		return notification.MobileDeliveryProjection{}, false, notification.ErrInvalidDelivery
	}
	var projection notification.MobileDeliveryProjection
	var channel string
	var state string
	var sensitivePreviewPolicy string
	err := c.db.QueryRowContext(ctx, `
		SELECT d.id::text,
		       i.id::text,
		       i.purpose,
		       'booking/' || i.booking_id::text,
		       d.channel,
		       d.state,
		       i.data_class,
		       i.sensitive_preview_policy
		FROM notification_deliveries d
		JOIN notification_intents i ON i.id = d.intent_id
		WHERE d.id::text = $1
		  AND i.recipient_identity_id::text = $2
		  AND d.channel = 'PUSH'
	`, deliveryID, identityID).Scan(
		&projection.DeliveryID,
		&projection.IntentID,
		&projection.Purpose,
		&projection.RelatedObjectRef,
		&channel,
		&state,
		&projection.DataClass,
		&sensitivePreviewPolicy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return notification.MobileDeliveryProjection{}, false, nil
	}
	if err != nil {
		return notification.MobileDeliveryProjection{}, false, err
	}
	projection.ContractVersion = "notification-projection-v1"
	projection.Channel = notification.Channel(channel)
	projection.DeliveryState = notification.DeliveryState(state)
	projection.PreviewMode = notification.PreviewModeFor(
		projection.DataClass,
		projection.Channel,
		strings.EqualFold(sensitivePreviewPolicy, "FULL_ALLOWED"),
	)
	return projection, true, nil
}
