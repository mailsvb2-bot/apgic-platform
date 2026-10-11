package notification

import (
	"context"
	"testing"
	"time"
)

type memoryDeliveryStore struct {
	plans      map[string]DeliveryPlan
	dispatches map[string]bool
}

func (s *memoryDeliveryStore) ClaimNotificationDelivery(_ context.Context, plan DeliveryPlan, _ DeliveryAudit) (DeliveryPlan, bool, error) {
	if s.plans == nil {
		s.plans = make(map[string]DeliveryPlan)
	}
	if existing, ok := s.plans[plan.IdempotencyKey]; ok {
		return existing, true, nil
	}
	s.plans[plan.IdempotencyKey] = plan
	return plan, false, nil
}

func (s *memoryDeliveryStore) ReserveNotificationPushDispatch(_ context.Context, plan DeliveryPlan, _, _ string) (PushTransportEnvelope, error) {
	if s.dispatches == nil {
		s.dispatches = make(map[string]bool)
	}
	if s.dispatches[plan.DeliveryID] {
		return PushTransportEnvelope{}, ErrPushDispatchAlreadyReserved
	}
	s.dispatches[plan.DeliveryID] = true
	return PushTransportEnvelope{
		ContractVersion: "notification-transport-v1",
		DeliveryID:      plan.DeliveryID,
		IntentID:        plan.IntentID,
	}, nil
}

func TestPushTransportIsOneIdempotentIntentWithSafePreview(t *testing.T) {
	policy, err := NewDeliveryPolicy("channel-policy-v1", []Channel{ChannelPush, ChannelEmail}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := NewTransactional(Intent{
		ID:             "intent-1",
		BookingID:      "booking-1",
		Purpose:        "BOOKING_CONFIRMATION",
		IdempotencyKey: "booking-1:confirmation",
		DataClass:      "SENSITIVE",
		CreatedAt:      time.Unix(1000, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryDeliveryStore{}
	service, err := NewTransportService(store)
	if err != nil {
		t.Fatal(err)
	}
	request := DeliveryRequest{
		DeliveryID:         "delivery-1",
		AuditID:            "audit-1",
		ActorID:            "system:notification",
		Channel:            ChannelPush,
		EndpointRef:        "device-1",
		ProviderInstanceID: "provider-1",
		OccurredAt:         time.Unix(1010, 0).UTC(),
	}
	first, duplicate, err := service.Claim(context.Background(), intent, request, policy)
	if err != nil || duplicate {
		t.Fatalf("first claim = %#v duplicate=%v err=%v", first, duplicate, err)
	}
	if first.IntentID != intent.ID || first.State != DeliveryPending || first.PreviewMode != PreviewGeneric {
		t.Fatalf("unsafe or split push plan: %#v", first)
	}
	if intent.RequiresGrowthOptIn() {
		t.Fatal("transactional push must not become dependent on growth opt-in")
	}
	dispatcher, err := NewPushDispatchService(store)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := dispatcher.Reserve(context.Background(), first, "dispatch-audit-1", "system:notification")
	if err != nil {
		t.Fatal(err)
	}
	if envelope.ContractVersion != "notification-transport-v1" || envelope.DeliveryID != first.DeliveryID || envelope.IntentID != intent.ID {
		t.Fatalf("unexpected push envelope: %#v", envelope)
	}

	request.DeliveryID = "delivery-retry"
	request.AuditID = "audit-2"
	second, duplicate, err := service.Claim(context.Background(), intent, request, policy)
	if err != nil || !duplicate || second.DeliveryID != first.DeliveryID {
		t.Fatalf("duplicate claim = %#v duplicate=%v err=%v", second, duplicate, err)
	}
	if _, err := dispatcher.Reserve(context.Background(), second, "dispatch-audit-2", "system:notification"); err != ErrPushDispatchAlreadyReserved {
		t.Fatalf("duplicate transport dispatch must be rejected: %v", err)
	}
}

func TestChannelPreferenceSuppressesBeforeProviderEnvelope(t *testing.T) {
	policy, err := NewDeliveryPolicy("channel-policy-v1", []Channel{ChannelEmail}, false, true)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := NewTransactional(Intent{
		ID:             "intent-2",
		BookingID:      "booking-2",
		Purpose:        "BOOKING_CONFIRMATION",
		IdempotencyKey: "booking-2:confirmation",
		DataClass:      "PUBLIC",
		CreatedAt:      time.Unix(2000, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, auditDecision, err := PlanDelivery(intent, DeliveryRequest{
		DeliveryID:         "delivery-2",
		AuditID:            "audit-2",
		ActorID:            "system:notification",
		Channel:            ChannelPush,
		EndpointRef:        "device-2",
		ProviderInstanceID: "provider-2",
		OccurredAt:         time.Unix(2010, 0).UTC(),
	}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if plan.State != DeliverySuppressed || plan.ReasonCode != ReasonDeliveryPreferenceSuppressed || auditDecision.ReasonCode != ReasonDeliveryPreferenceSuppressed {
		t.Fatalf("preference was not applied before provider: plan=%#v audit=%#v", plan, auditDecision)
	}
	dispatcher, err := NewPushDispatchService(&memoryDeliveryStore{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dispatcher.Reserve(context.Background(), plan, "dispatch-audit-2", "system:notification"); err == nil {
		t.Fatal("suppressed delivery must not produce provider push envelope")
	}
}

func TestPushDispatchRejectsNonPendingDeliveryStates(t *testing.T) {
	dispatcher, err := NewPushDispatchService(&memoryDeliveryStore{})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []DeliveryState{
		DeliverySent,
		DeliveryDelivered,
		DeliveryRetryable,
		DeliverySuppressed,
		DeliveryState("UNKNOWN"),
		"",
	} {
		t.Run(string(state), func(t *testing.T) {
			plan := DeliveryPlan{
				DeliveryID: "delivery-1",
				IntentID:   "intent-1",
				Channel:    ChannelPush,
				State:      state,
			}
			if _, err := dispatcher.Reserve(context.Background(), plan, "audit-1", "system:notification"); err == nil {
				t.Fatalf("state %q must not create a fresh provider push envelope", state)
			}
		})
	}
	for _, tc := range []DeliveryPlan{
		{DeliveryID: "", IntentID: "intent-1", Channel: ChannelPush, State: DeliveryPending},
		{DeliveryID: "delivery-1", IntentID: "", Channel: ChannelPush, State: DeliveryPending},
		{DeliveryID: "delivery-1", IntentID: "intent-1", Channel: ChannelEmail, State: DeliveryPending},
	} {
		if _, err := dispatcher.Reserve(context.Background(), tc, "audit-2", "system:notification"); err == nil {
			t.Fatalf("invalid pending push scope must be rejected: %#v", tc)
		}
	}
}
