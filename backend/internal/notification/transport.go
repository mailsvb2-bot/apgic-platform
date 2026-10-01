package notification

import (
	"context"
	"errors"
	"strings"
	"time"
)

type DeliveryState string

type PreviewMode string

const (
	DeliveryPending    DeliveryState = "PENDING"
	DeliverySent       DeliveryState = "SENT"
	DeliveryDelivered  DeliveryState = "DELIVERED"
	DeliveryRetryable  DeliveryState = "FAILED_RETRYABLE"
	DeliverySuppressed DeliveryState = "SUPPRESSED"

	PreviewGeneric PreviewMode = "GENERIC"
	PreviewFull    PreviewMode = "FULL"
)

const (
	ReasonDeliveryAllowed              = "NOTIFICATION_DELIVERY_ALLOWED"
	ReasonDeliveryPreferenceSuppressed = "NOTIFICATION_DELIVERY_PREFERENCE_SUPPRESSED"
	ReasonDeliveryDuplicate            = "NOTIFICATION_DELIVERY_DUPLICATE"
	ReasonDeliveryIdempotencyConflict  = "NOTIFICATION_DELIVERY_IDEMPOTENCY_CONFLICT"
)

var (
	ErrInvalidDelivery             = errors.New("invalid notification delivery")
	ErrDeliveryIdempotencyConflict = errors.New("notification delivery idempotency conflict")
)

type DeliveryPolicy struct {
	Version               string
	EnabledChannels       map[Channel]bool
	AllowSensitivePreview bool
	GrowthOptIn           bool
}

func NewDeliveryPolicy(version string, enabled []Channel, allowSensitivePreview, growthOptIn bool) (DeliveryPolicy, error) {
	if strings.TrimSpace(version) == "" {
		return DeliveryPolicy{}, ErrInvalidDelivery
	}
	channels := make(map[Channel]bool, len(enabled))
	for _, channel := range enabled {
		if !validChannel(channel) {
			return DeliveryPolicy{}, ErrInvalidDelivery
		}
		channels[channel] = true
	}
	return DeliveryPolicy{
		Version:               strings.TrimSpace(version),
		EnabledChannels:       channels,
		AllowSensitivePreview: allowSensitivePreview,
		GrowthOptIn:           growthOptIn,
	}, nil
}

type DeliveryRequest struct {
	DeliveryID         string
	AuditID            string
	ActorID            string
	Channel            Channel
	EndpointRef        string
	ProviderInstanceID string
	OccurredAt         time.Time
}

type DeliveryPlan struct {
	DeliveryID         string
	IntentID           string
	BookingID          string
	DataClass          string
	Channel            Channel
	EndpointRef        string
	ProviderInstanceID string
	IdempotencyKey     string
	State              DeliveryState
	PreviewMode        PreviewMode
	ReasonCode         string
	PolicyVersion      string
	OccurredAt         time.Time
}

type DeliveryAudit struct {
	ID            string
	ActorID       string
	ReasonCode    string
	PolicyVersion string
	CorrelationID string
	OccurredAt    time.Time
}

type PushTransportEnvelope struct {
	ContractVersion string `json:"contract_version"`
	DeliveryID      string `json:"delivery_id"`
	IntentID        string `json:"intent_id"`
}

type MobileDeliveryProjection struct {
	ContractVersion  string        `json:"contract_version"`
	DeliveryID       string        `json:"delivery_id"`
	IntentID         string        `json:"intent_id"`
	Purpose          string        `json:"purpose"`
	RelatedObjectRef string        `json:"related_object_ref"`
	Channel          Channel       `json:"channel"`
	DeliveryState    DeliveryState `json:"delivery_state"`
	DataClass        string        `json:"data_class"`
	PreviewMode      PreviewMode   `json:"preview_mode"`
}

type DeliveryStore interface {
	ClaimNotificationDelivery(context.Context, DeliveryPlan, DeliveryAudit) (DeliveryPlan, bool, error)
}

type MobileProjectionStore interface {
	MobileNotificationDelivery(context.Context, string, string) (MobileDeliveryProjection, bool, error)
}

type TransportService struct {
	store DeliveryStore
}

func NewTransportService(store DeliveryStore) (*TransportService, error) {
	if store == nil {
		return nil, ErrInvalidDelivery
	}
	return &TransportService{store: store}, nil
}

func (s *TransportService) Claim(ctx context.Context, intent Intent, request DeliveryRequest, policy DeliveryPolicy) (DeliveryPlan, bool, error) {
	plan, decision, err := PlanDelivery(intent, request, policy)
	if err != nil {
		return DeliveryPlan{}, false, err
	}
	return s.store.ClaimNotificationDelivery(ctx, plan, decision)
}

func PlanDelivery(intent Intent, request DeliveryRequest, policy DeliveryPolicy) (DeliveryPlan, DeliveryAudit, error) {
	if !intent.Transactional || strings.TrimSpace(intent.ID) == "" || strings.TrimSpace(intent.BookingID) == "" ||
		strings.TrimSpace(intent.DataClass) == "" || strings.TrimSpace(request.DeliveryID) == "" ||
		strings.TrimSpace(request.AuditID) == "" || strings.TrimSpace(request.ActorID) == "" ||
		!validChannel(request.Channel) || strings.TrimSpace(request.EndpointRef) == "" ||
		strings.TrimSpace(request.ProviderInstanceID) == "" || request.OccurredAt.IsZero() ||
		strings.TrimSpace(policy.Version) == "" {
		return DeliveryPlan{}, DeliveryAudit{}, ErrInvalidDelivery
	}
	idempotencyKey, err := DeliveryIdempotencyKey(intent, request.Channel, request.EndpointRef)
	if err != nil {
		return DeliveryPlan{}, DeliveryAudit{}, err
	}
	state := DeliveryPending
	reason := ReasonDeliveryAllowed
	if !policy.EnabledChannels[request.Channel] {
		state = DeliverySuppressed
		reason = ReasonDeliveryPreferenceSuppressed
	}
	preview := PreviewModeFor(intent.DataClass, request.Channel, policy.AllowSensitivePreview)
	plan := DeliveryPlan{
		DeliveryID:         strings.TrimSpace(request.DeliveryID),
		IntentID:           strings.TrimSpace(intent.ID),
		BookingID:          strings.TrimSpace(intent.BookingID),
		DataClass:          strings.TrimSpace(intent.DataClass),
		Channel:            request.Channel,
		EndpointRef:        strings.TrimSpace(request.EndpointRef),
		ProviderInstanceID: strings.TrimSpace(request.ProviderInstanceID),
		IdempotencyKey:     idempotencyKey,
		State:              state,
		PreviewMode:        preview,
		ReasonCode:         reason,
		PolicyVersion:      strings.TrimSpace(policy.Version),
		OccurredAt:         request.OccurredAt.UTC(),
	}
	decision := DeliveryAudit{
		ID:            strings.TrimSpace(request.AuditID),
		ActorID:       strings.TrimSpace(request.ActorID),
		ReasonCode:    reason,
		PolicyVersion: strings.TrimSpace(policy.Version),
		CorrelationID: idempotencyKey,
		OccurredAt:    request.OccurredAt.UTC(),
	}
	return plan, decision, nil
}

func PushEnvelope(plan DeliveryPlan) (PushTransportEnvelope, error) {
	if plan.Channel != ChannelPush || strings.TrimSpace(plan.DeliveryID) == "" || strings.TrimSpace(plan.IntentID) == "" || plan.State == DeliverySuppressed {
		return PushTransportEnvelope{}, ErrInvalidDelivery
	}
	return PushTransportEnvelope{
		ContractVersion: "notification-transport-v1",
		DeliveryID:      plan.DeliveryID,
		IntentID:        plan.IntentID,
	}, nil
}

func PreviewModeFor(dataClass string, channel Channel, allowSensitivePreview bool) PreviewMode {
	if channel != ChannelPush {
		return PreviewFull
	}
	class := strings.ToUpper(strings.TrimSpace(dataClass))
	if !allowSensitivePreview && class != "PUBLIC" && class != "INTERNAL" {
		return PreviewGeneric
	}
	return PreviewFull
}

func validChannel(channel Channel) bool {
	return channel == ChannelPush || channel == ChannelEmail || channel == ChannelSMS
}
