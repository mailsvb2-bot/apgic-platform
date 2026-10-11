package runtimepostgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/notification"
)

// ReserveNotificationPushDispatch is the irreversible dispatch boundary.
// A reservation is NOT proof of delivery; ambiguous external outcomes are
// reconciled, never silently retried with another physical provider send.
func (c *Checker) ReserveNotificationPushDispatch(ctx context.Context, plan notification.DeliveryPlan, auditID, actorID string) (notification.PushTransportEnvelope, error) {
	if c == nil || c.db == nil {
		return notification.PushTransportEnvelope{}, notification.ErrPushDispatchUnavailable
	}
	if plan.Channel != notification.ChannelPush || plan.State != notification.DeliveryPending ||
		strings.TrimSpace(plan.DeliveryID) == "" || strings.TrimSpace(plan.IntentID) == "" ||
		strings.TrimSpace(plan.ProviderInstanceID) == "" || strings.TrimSpace(plan.BookingID) == "" ||
		strings.TrimSpace(plan.IdempotencyKey) == "" || strings.TrimSpace(plan.EndpointRef) == "" ||
		strings.TrimSpace(plan.DataClass) == "" || strings.TrimSpace(plan.PolicyVersion) == "" ||
		strings.TrimSpace(auditID) == "" || strings.TrimSpace(actorID) == "" {
		return notification.PushTransportEnvelope{}, notification.ErrInvalidDelivery
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return notification.PushTransportEnvelope{}, err
	}
	defer tx.Rollback()

	// The reservation insert is the linearization point for every competing
	// worker, including a worker holding an old in-memory PENDING snapshot.
	// Provider/intent/policy/time-window scope is read from authoritative rows.
	var reservedDelivery string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO notification_push_dispatches (delivery_id, reserved_at)
		SELECT d.id, clock_timestamp()
		FROM notification_deliveries d
		JOIN notification_intents i ON i.id = d.intent_id
		WHERE d.id = $1::uuid
		  AND d.intent_id = $2::uuid
		  AND d.provider_instance_id = $3::uuid
		  AND d.delivery_idempotency_key = $4
		  AND d.endpoint_ref = $5
		  AND i.booking_id = $6::uuid
		  AND i.data_class = $7
		  AND i.channel_policy_version = $8
		  AND d.channel = 'PUSH'
		  AND d.state = 'PENDING'
		  AND COALESCE((i.channel_preferences ->> 'PUSH')::boolean, false)
		  AND clock_timestamp() >= i.not_before
		  AND (i.expires_at IS NULL OR clock_timestamp() < i.expires_at)
		ON CONFLICT (delivery_id) DO NOTHING
		RETURNING delivery_id::text
	`, plan.DeliveryID, plan.IntentID, plan.ProviderInstanceID, plan.IdempotencyKey,
		plan.EndpointRef, plan.BookingID, plan.DataClass, plan.PolicyVersion).Scan(&reservedDelivery)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return notification.PushTransportEnvelope{}, err
	}

	var reason string
	var denied error
	if errors.Is(err, sql.ErrNoRows) {
		var alreadyReserved bool
		if err = tx.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM notification_push_dispatches WHERE delivery_id = $1::uuid
			)
		`, plan.DeliveryID).Scan(&alreadyReserved); err != nil {
			return notification.PushTransportEnvelope{}, err
		}
		if alreadyReserved {
			reason = notification.ReasonPushDispatchDuplicate
			denied = notification.ErrPushDispatchAlreadyReserved
		} else {
			reason = notification.ReasonPushDispatchDenied
			denied = notification.ErrPushDispatchNotAllowed
		}
	} else {
		reason = notification.ReasonPushDispatchReserved
	}

	state, err := json.Marshal(map[string]any{
		"delivery_id":        plan.DeliveryID,
		"intent_id":          plan.IntentID,
		"reservation_reason": reason,
	})
	if err != nil {
		return notification.PushTransportEnvelope{}, err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO audit_records (
			id, actor_id, action, scope, resource_ref, new_state,
			reason, policy_version, correlation_id, occurred_at
		) VALUES (
			$1::uuid, $2, 'NOTIFICATION_PUSH_DISPATCH_DECISION',
			'notification.delivery', $3, $4::jsonb,
			$5, $6, $7, clock_timestamp()
		)
	`, auditID, actorID, "notification-delivery/"+plan.DeliveryID,
		string(state), reason, plan.PolicyVersion, plan.IdempotencyKey); err != nil {
		return notification.PushTransportEnvelope{}, err
	}
	if err = tx.Commit(); err != nil {
		return notification.PushTransportEnvelope{}, err
	}
	if denied != nil {
		return notification.PushTransportEnvelope{}, denied
	}
	return notification.PushTransportEnvelope{
		ContractVersion: "notification-transport-v1",
		DeliveryID:      reservedDelivery,
		IntentID:        plan.IntentID,
	}, nil
}
