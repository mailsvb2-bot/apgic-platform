package runtimepostgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/notification"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

func TestNotificationTransportClaimIsIdempotentAuditedAndOwned(t *testing.T) {
	databaseURL := os.Getenv("APGIC_NOTIFICATION_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("notification integration database not configured")
	}
	store, err := Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	const (
		intentID   = "00000000-0000-0000-0000-00000000f101"
		bookingID  = "00000000-0000-0000-0000-00000000b301"
		providerID = "00000000-0000-0000-0000-00000000f001"
	)
	var ownerIdentity string
	if err := store.db.QueryRow(`SELECT client_identity_id::text FROM bookings WHERE id = $1::uuid`, bookingID).Scan(&ownerIdentity); err != nil {
		t.Fatal(err)
	}

	policy, err := notification.NewDeliveryPolicy("channel-policy-ci-v1", []notification.Channel{notification.ChannelEmail}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	service, err := notification.NewTransportService(store)
	if err != nil {
		t.Fatal(err)
	}
	intent := notification.Intent{
		ID:             intentID,
		BookingID:      bookingID,
		Purpose:        "BOOKING_CONFIRMATION",
		IdempotencyKey: "booking-b301:confirmation",
		Transactional:  true,
		DataClass:      "SENSITIVE",
		CreatedAt:      time.Now().UTC(),
	}
	deliveryID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	auditID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	endpoint := "device/runtime-" + deliveryID
	occurredAt := time.Now().UTC().Add(time.Minute)
	request := notification.DeliveryRequest{
		DeliveryID:         deliveryID,
		AuditID:            auditID,
		ActorID:            "system:notification",
		Channel:            notification.ChannelPush,
		EndpointRef:        endpoint,
		ProviderInstanceID: providerID,
		OccurredAt:         occurredAt,
	}

	first, duplicate, err := service.Claim(context.Background(), intent, request, policy)
	if err != nil || duplicate {
		t.Fatalf("first claim=%#v duplicate=%v err=%v", first, duplicate, err)
	}
	// The caller cache says PUSH is disabled; canonical PostgreSQL preferences for this intent say PUSH=true.
	// Provider-side truth must win before any side effect.
	if first.State != notification.DeliveryPending || first.PreviewMode != notification.PreviewGeneric {
		t.Fatalf("unsafe first push plan: %#v", first)
	}
	if _, err := notification.PushEnvelope(first); err != nil {
		t.Fatal(err)
	}

	retryDeliveryID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	retryAuditID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	request.DeliveryID = retryDeliveryID
	request.AuditID = retryAuditID
	request.OccurredAt = occurredAt.Add(time.Second)
	second, duplicate, err := service.Claim(context.Background(), intent, request, policy)
	if err != nil || !duplicate || second.DeliveryID != first.DeliveryID {
		t.Fatalf("retry claim=%#v duplicate=%v err=%v", second, duplicate, err)
	}

	projection, found, err := store.MobileNotificationDelivery(context.Background(), ownerIdentity, first.DeliveryID)
	if err != nil || !found {
		t.Fatalf("owner projection=%#v found=%v err=%v", projection, found, err)
	}
	if projection.IntentID != intentID || projection.Channel != notification.ChannelPush ||
		projection.PreviewMode != notification.PreviewGeneric || projection.DataClass != "SENSITIVE" {
		t.Fatalf("unsafe owner projection: %#v", projection)
	}
	if _, found, err := store.MobileNotificationDelivery(context.Background(), "00000000-0000-0000-0000-00000000ffff", first.DeliveryID); err != nil || found {
		t.Fatalf("cross-user projection leaked: found=%v err=%v", found, err)
	}

	var deliveryCount int
	if err := store.db.QueryRow(`SELECT count(*) FROM notification_deliveries WHERE delivery_idempotency_key = $1`, first.IdempotencyKey).Scan(&deliveryCount); err != nil {
		t.Fatal(err)
	}
	if deliveryCount != 1 {
		t.Fatalf("duplicate push created %d provider delivery rows", deliveryCount)
	}
	var auditCount int
	if err := store.db.QueryRow(`SELECT count(*) FROM audit_records WHERE correlation_id = $1 AND action = 'NOTIFICATION_DELIVERY_DECISION'`, first.IdempotencyKey).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 2 {
		t.Fatalf("delivery attempts must both be auditable, got %d records", auditCount)
	}
}
