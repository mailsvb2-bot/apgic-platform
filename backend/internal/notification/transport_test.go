package notification

import (
	"context"
	"testing"
	"time"
)

type memoryDeliveryStore struct {
	plans map[string]DeliveryPlan
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

func TestPushTransportIsOneIdempotentIntentWithSafePreview(t *testing.T) {
	policy, err := NewDeliveryPolicy("channel-policy-v1", []Channel{ChannelPush, ChannelEmail}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := NewTransactional(Intent{
		ID: "intent-1", BookingID: "booking-1", Purpose: "BOOKING_CONFIRMATION",
		IdempotencyKey: "booking-1:confirmation", DataClass: "SENSITIVE",
		CreatedAt: time.Unix(1000, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewTransportService(&memoryDeliveryStore{})
	if err != nil {
		t.Fatal(err)
	}
	request := DeliveryRequest{
		DeliveryID: "delivery-1", AuditID: "audit-1", ActorID: "system:notification",
		Channel: ChannelPush, EndpointRef: "device-1", ProviderInstanceID: "provider-1",
		OccurredAt: time.Unix(1010, 0).UTC(),
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
	envelope, err := PushEnvelope(first)
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
}

func TestChannelPreferenceSuppressesBeforeProviderEnvelope(t *testing.T) {
	policy, err := NewDeliveryPolicy("channel-policy-v1", []Channel{ChannelEmail}, false, true)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := NewTransactional(Intent{
		ID: "intent-2", BookingID: "booking-2", Purpose: "BOOKING_CONFIRMATION",
		IdempotencyKey: "booking-2:confirmation", DataClass: "PUBLIC",
		CreatedAt: time.Unix(2000, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, auditDecision, err := PlanDelivery(intent, DeliveryRequest{
		DeliveryID: "delivery-2", AuditID: "audit-2", ActorID: "system:notification",
		Channel: ChannelPush, EndpointRef: "device-2", ProviderInstanceID: "provider-2",
		OccurredAt: time.Unix(2010, 0).UTC(),
	}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if plan.State != DeliverySuppressed || plan.ReasonCode != ReasonDeliveryPreferenceSuppressed ||
		auditDecision.ReasonCode != ReasonDeliveryPreferenceSuppressed {
		t.Fatalf("preference was not applied before provider: plan=%#v audit=%#v", plan, auditDecision)
	}
	if _, err := PushEnvelope(plan); err == nil {
		t.Fatal("suppressed delivery must not produce provider push envelope")
	}
}
