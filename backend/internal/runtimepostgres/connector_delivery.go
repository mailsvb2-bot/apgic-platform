package runtimepostgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/connector"
)

type ConnectorDomainEffect func(context.Context, *sql.Tx, json.RawMessage) error

func (c *Checker) ProcessConnectorWebhook(
	ctx context.Context,
	envelope connector.WebhookEnvelope,
	resolver connector.WebhookPublicKeyResolver,
	apply ConnectorDomainEffect,
) (connector.DeliveryDecision, error) {
	if c == nil || c.db == nil {
		return "", errors.New("postgres checker is not initialized")
	}
	if apply == nil {
		return "", errors.New("connector domain effect is required")
	}
	if err := connector.VerifyWebhook(envelope, resolver); err != nil {
		return "", err
	}

	payload, err := canonicalConnectorPayload(envelope.Payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin connector delivery: %w", err)
	}
	defer tx.Rollback()

	var decision string
	if err := tx.QueryRowContext(ctx,
		`SELECT apgic_register_connector_delivery(
			$1::uuid, $2, $3, $4, $5::jsonb, $6, $7
		)`,
		envelope.ConnectorInstanceID,
		envelope.ExternalEventID,
		envelope.StreamID,
		envelope.Sequence,
		string(payload),
		hex.EncodeToString(sum[:]),
		envelope.KeyID,
	).Scan(&decision); err != nil {
		return "", fmt.Errorf("register connector delivery: %w", err)
	}

	switch connector.DeliveryDecision(decision) {
	case connector.DeliveryApply:
		if err := apply(ctx, tx, append(json.RawMessage(nil), payload...)); err != nil {
			return "", fmt.Errorf("apply connector domain effect: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`SELECT apgic_finalize_connector_delivery($1::uuid, $2)`,
			envelope.ConnectorInstanceID,
			envelope.ExternalEventID,
		); err != nil {
			return "", fmt.Errorf("finalize connector delivery: %w", err)
		}
	case connector.DeliveryDuplicate, connector.DeliveryStale, connector.DeliveryDefer:
	default:
		return "", fmt.Errorf("unknown connector delivery decision %q", decision)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit connector delivery: %w", err)
	}
	return connector.DeliveryDecision(decision), nil
}

func (c *Checker) ApplyNextDeferredConnectorWebhook(
	ctx context.Context,
	connectorInstanceID,
	streamID string,
	apply ConnectorDomainEffect,
) (string, bool, error) {
	if c == nil || c.db == nil {
		return "", false, errors.New("postgres checker is not initialized")
	}
	if apply == nil {
		return "", false, errors.New("connector domain effect is required")
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return "", false, fmt.Errorf("begin deferred connector delivery: %w", err)
	}
	defer tx.Rollback()

	var externalEventID sql.NullString
	if err := tx.QueryRowContext(ctx,
		`SELECT apgic_promote_next_connector_delivery($1::uuid, $2)`,
		connectorInstanceID,
		streamID,
	).Scan(&externalEventID); err != nil {
		return "", false, fmt.Errorf("promote deferred connector delivery: %w", err)
	}
	if !externalEventID.Valid {
		if err := tx.Commit(); err != nil {
			return "", false, fmt.Errorf("commit empty deferred connector delivery: %w", err)
		}
		return "", false, nil
	}

	var payloadText string
	if err := tx.QueryRowContext(ctx,
		`SELECT payload::text
		   FROM connector_delivery_receipts
		  WHERE connector_instance_id = $1::uuid
		    AND external_event_id = $2
		    AND state = 'READY'
		  FOR UPDATE`,
		connectorInstanceID,
		externalEventID.String,
	).Scan(&payloadText); err != nil {
		return "", false, fmt.Errorf("read deferred connector payload: %w", err)
	}
	payload := json.RawMessage(payloadText)
	if !json.Valid(payload) {
		return "", false, errors.New("durable connector payload is invalid")
	}

	if err := apply(ctx, tx, append(json.RawMessage(nil), payload...)); err != nil {
		return "", false, fmt.Errorf("apply deferred connector domain effect: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`SELECT apgic_finalize_connector_delivery($1::uuid, $2)`,
		connectorInstanceID,
		externalEventID.String,
	); err != nil {
		return "", false, fmt.Errorf("finalize deferred connector delivery: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", false, fmt.Errorf("commit deferred connector delivery: %w", err)
	}
	return externalEventID.String, true, nil
}

func canonicalConnectorPayload(payload json.RawMessage) (json.RawMessage, error) {
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return nil, connector.ErrInvalidWebhook
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("canonicalize connector payload: %w", err)
	}
	return canonical, nil
}
