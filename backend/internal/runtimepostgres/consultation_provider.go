package runtimepostgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/connector"
)

var ErrConsultationProviderPayload = errors.New("invalid consultation provider event")

type ConsultationProviderEvent struct {
	BookingID         string `json:"booking_id"`
	FactType          string `json:"fact_type"`
	Role              string `json:"role"`
	ProviderReference string `json:"provider_reference"`
	EvidenceRef       string `json:"evidence_ref"`
}

func (c *Checker) ApplyConsultationProviderWebhook(ctx context.Context, envelope connector.WebhookEnvelope, keys connector.WebhookPublicKeyResolver) (connector.DeliveryDecision, error) {
	var body ConsultationProviderEvent
	if err := json.Unmarshal(envelope.Payload, &body); err != nil {
		return "", ErrConsultationProviderPayload
	}
	if strings.TrimSpace(body.BookingID) == "" || envelope.StreamID != "consultation/"+body.BookingID ||
		strings.TrimSpace(body.ProviderReference) == "" || strings.TrimSpace(body.EvidenceRef) == "" {
		return "", ErrConsultationProviderPayload
	}
	switch body.FactType {
	case "JOINED":
		if body.Role != "CLIENT" && body.Role != "SPECIALIST" {
			return "", ErrConsultationProviderPayload
		}
	case "STARTED", "ENDED":
		if body.Role != "SYSTEM" {
			return "", ErrConsultationProviderPayload
		}
	default:
		return "", ErrConsultationProviderPayload
	}
	return c.ProcessConnectorWebhook(ctx, envelope, keys, func(ctx context.Context, tx *sql.Tx, payload json.RawMessage) error {
		// This is executed in the same transaction as the signed delivery receipt.
		// PostgreSQL's consultation guard verifies booking, provider and transitions.
		result, err := tx.ExecContext(ctx, `INSERT INTO consultation_sessions
            (id, booking_id, client_identity_id, specialist_identity_id, provider_instance_id, state, created_at, updated_at)
            SELECT gen_random_uuid(), b.id, b.client_identity_id, bs.specialist_identity_id,
                   $2::uuid, 'SCHEDULED', $3, $3
            FROM bookings b JOIN booking_slots bs ON bs.id = b.slot_id
            JOIN connector_instances ci ON ci.id = $2::uuid
            WHERE b.id = $1::uuid AND b.state = 'CONFIRMED'
              AND ci.capability_class = 'COMMUNICATION_PROVIDER' AND ci.status IN ('ACTIVE','DEGRADED')
            ON CONFLICT (booking_id) DO NOTHING`,
			body.BookingID, envelope.ConnectorInstanceID, envelope.OccurredAt.UTC())
		if err != nil {
			return fmt.Errorf("consultation session insert: %w", err)
		}
		_ = result
		var sessionID, activeProviderID string
		var clientID, specialistID string
		err = tx.QueryRowContext(ctx, `SELECT s.id, s.provider_instance_id, s.client_identity_id, s.specialist_identity_id
            FROM consultation_sessions s JOIN bookings b ON b.id=s.booking_id
            WHERE s.booking_id=$1::uuid AND b.state='CONFIRMED' FOR UPDATE OF s`, body.BookingID).
			Scan(&sessionID, &activeProviderID, &clientID, &specialistID)
		if err != nil {
			return fmt.Errorf("confirmed consultation unavailable: %w", err)
		}
		if activeProviderID != envelope.ConnectorInstanceID {
			return ErrConsultationProviderPayload
		}
		var participant any
		if body.Role == "CLIENT" {
			participant = clientID
		}
		if body.Role == "SPECIALIST" {
			participant = specialistID
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO consultation_lifecycle_facts
            (id, session_id, fact_type, role, identity_id, provider_instance_id,
             provider_reference, evidence_ref, idempotency_key, occurred_at)
             VALUES(gen_random_uuid(), $1::uuid, $2, $3, $4::uuid, $5::uuid, $6, $7, $8, $9)`,
			sessionID, body.FactType, body.Role, participant, envelope.ConnectorInstanceID,
			body.ProviderReference, body.EvidenceRef, envelope.ExternalEventID, envelope.OccurredAt.UTC())
		if err != nil {
			return fmt.Errorf("apply consultation lifecycle fact: %w", err)
		}
		return nil
	})
}

func (c *Checker) ReadConsultationResult(ctx context.Context, bookingID, clientIdentityID string) (*connector.ConsultationResult, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("consultation storage is unavailable")
	}
	var result connector.ConsultationResult
	var evidence sql.NullString
	err := c.db.QueryRowContext(ctx, `SELECT s.booking_id, s.state, s.provider_instance_id,
        (SELECT f.evidence_ref FROM consultation_lifecycle_facts f
          WHERE f.session_id=s.id AND f.fact_type='ENDED'
          ORDER BY f.occurred_at DESC, f.id DESC LIMIT 1)
        FROM consultation_sessions s JOIN bookings b ON b.id=s.booking_id
        WHERE s.booking_id=$1::uuid AND b.client_identity_id=$2::uuid`,
		bookingID, clientIdentityID).Scan(&result.BookingID, &result.State, &result.ProviderInstanceID, &evidence)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read consultation result: %w", err)
	}
	if evidence.Valid {
		result.CompletionEvidenceRef = evidence.String
	}
	return &result, nil
}