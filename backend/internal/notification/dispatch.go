package notification

import (
	"context"
	"errors"
	"strings"
)

// A dispatch reservation is not a send receipt. After a provider timeout its
// outcome is UNKNOWN until authoritative reconciliation; do not replay it.
var (
	ErrPushDispatchUnavailable = errors.New("push dispatch store is unavailable")
	ErrPushDispatchAlreadyReserved = errors.New("push dispatch already reserved; reconcile provider outcome before retry")
	ErrPushDispatchNotAllowed = errors.New("push dispatch denied by canonical delivery policy")
)

const (
	ReasonPushDispatchReserved = "NOTIFICATION_PUSH_DISPATCH_RESERVED"
	ReasonPushDispatchDuplicate = "NOTIFICATION_PUSH_DISPATCH_DUPLICATE"
	ReasonPushDispatchDenied = "NOTIFICATION_PUSH_DISPATCH_DENIED"
)

// PushDispatchStore is the only authority permitted to produce an outbound
// transport envelope: it must atomically reserve the canonical delivery in
// durable storage before returning the envelope to the provider adapter.
type PushDispatchStore interface {
	ReserveNotificationPushDispatch(context.Context, DeliveryPlan, string, string) (PushTransportEnvelope, error)
}

type PushDispatchService struct {
	store PushDispatchStore
}

func NewPushDispatchService(store PushDispatchStore) (*PushDispatchService, error) {
	if store == nil {
		return nil, ErrPushDispatchUnavailable
	}
	return &PushDispatchService{store: store}, nil
}

// Reserve returns a provider-ready envelope only after a successful durable
// one-shot reservation. ClaimNotificationDelivery alone never authorizes send.
func (s *PushDispatchService) Reserve(ctx context.Context, plan DeliveryPlan, auditID, actorID string) (PushTransportEnvelope, error) {
	if s == nil || s.store == nil {
		return PushTransportEnvelope{}, ErrPushDispatchUnavailable
	}
	if plan.Channel != ChannelPush || plan.State != DeliveryPending ||
		strings.TrimSpace(plan.DeliveryID) == "" || strings.TrimSpace(plan.IntentID) == "" ||
		strings.TrimSpace(plan.BookingID) == "" || strings.TrimSpace(plan.ProviderInstanceID) == "" ||
		strings.TrimSpace(plan.IdempotencyKey) == "" || strings.TrimSpace(plan.EndpointRef) == "" ||
		strings.TrimSpace(plan.DataClass) == "" || strings.TrimSpace(plan.PolicyVersion) == "" ||
		strings.TrimSpace(auditID) == "" || strings.TrimSpace(actorID) == "" {
		return PushTransportEnvelope{}, ErrInvalidDelivery
	}
	return s.store.ReserveNotificationPushDispatch(ctx, plan, auditID, actorID)
}
