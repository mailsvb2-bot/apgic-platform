package eventspine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const EventDeliveryScope = "event:deliver"

var (
	ErrDeliveryConfigInvalid = errors.New("event gateway delivery configuration is invalid")
	ErrDeliveryRejected      = errors.New("event gateway rejected outbox event")
)

type HTTPDeliverer struct {
	endpoint    string
	principalID string
	credential  string
	client      *http.Client
}

type wireEventEnvelope struct {
	EventID          string          `json:"event_id"`
	IdempotencyKey   string          `json:"idempotency_key"`
	EventType        string          `json:"event_type"`
	SchemaVersion    string          `json:"schema_version"`
	AggregateRef     string          `json:"aggregate_ref"`
	AggregateVersion uint64          `json:"aggregate_version,omitempty"`
	OccurredAt       time.Time       `json:"occurred_at"`
	ProducedAt       time.Time       `json:"produced_at"`
	Producer         string          `json:"producer"`
	TenantScope      string          `json:"tenant_scope,omitempty"`
	CorrelationID    string          `json:"correlation_id"`
	CausationID      string          `json:"causation_id,omitempty"`
	Payload          json.RawMessage `json:"payload"`
}

func NewHTTPDeliverer(endpoint, principalID, credential string, client *http.Client) (*HTTPDeliverer, error) {
	endpoint = strings.TrimSpace(endpoint)
	principalID = strings.TrimSpace(principalID)
	if endpoint == "" || principalID == "" || len([]byte(credential)) < 32 {
		return nil, ErrDeliveryConfigInvalid
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return nil, ErrDeliveryConfigInvalid
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &HTTPDeliverer{
		endpoint: endpoint, principalID: principalID, credential: credential, client: client,
	}, nil
}

func (d *HTTPDeliverer) Deliver(ctx context.Context, event EventEnvelope) error {
	if d == nil {
		return ErrDeliveryConfigInvalid
	}
	record, err := NewRecord(event)
	if err != nil {
		return err
	}
	event = record.Event
	payload, err := json.Marshal(wireEventEnvelope{
		EventID: event.EventID,
		IdempotencyKey: event.IdempotencyKey,
		EventType: event.EventType,
		SchemaVersion: event.SchemaVersion,
		AggregateRef: event.AggregateRef,
		AggregateVersion: event.AggregateVersion,
		OccurredAt: event.OccurredAt.UTC(),
		ProducedAt: event.ProducedAt.UTC(),
		Producer: event.Producer,
		TenantScope: event.TenantScope,
		CorrelationID: event.CorrelationID,
		CausationID: event.CausationID,
		Payload: json.RawMessage(event.PayloadJSON),
	})
	if err != nil {
		return fmt.Errorf("encode outbox delivery: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create outbox delivery request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", event.IdempotencyKey)
	req.Header.Set("X-APGIC-Service-Principal", d.principalID)
	req.Header.Set("X-APGIC-Service-Scope", EventDeliveryScope)
	req.Header.Set("Authorization", "Bearer "+d.credential)

	response, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("deliver outbox event: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%w: status=%d", ErrDeliveryRejected, response.StatusCode)
	}
	return nil
}
