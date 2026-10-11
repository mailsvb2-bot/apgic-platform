package runtimepostgres

import (
	"context"
	"errors"
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
	// Eight concurrent workers must not obtain eight provider-ready envelopes,
	// even if all retain the exact same stale PENDING delivery plan.
	dispatcher, err := notification.NewPushDispatchService(store)
	if err != nil {
		t.Fatal(err)
	}
	const workers = 8
	auditIDs := make([]string, workers)
	for i := range auditIDs {
		auditIDs[i], err = persistentid.New()
		if err != nil {
			t.Fatal(err)
		}
	}
	type reservationResult struct {
		envelope notification.PushTransportEnvelope
		err error
	}
	results := make(chan reservationResult, workers)
	for _, id := range auditIDs {
		go func(auditID string) {
			envelope, dispatchErr := dispatcher.Reserve(context.Background(), first, auditID, "system:notification")
			results <- reservationResult{envelope: envelope, err: dispatchErr}
		}(id)
	}
	var granted, blocked int
	for i := 0; i < workers; i++ {
		result := <-results
		switch {
		case result.err == nil:
			granted++
			if result.envelope.DeliveryID != first.DeliveryID || result.envelope.IntentID != intentID {
				t.Fatalf("dispatch returned incorrect provider scope: %#v", result.envelope)
			}
		case errors.Is(result.err, notification.ErrPushDispatchAlreadyReserved):
			blocked++
			if result.envelope.DeliveryID != "" {
				t.Fatal("duplicate reservation leaked a provider-ready envelope")
			}
		default:
			t.Fatalf("concurrent reservation failed unexpectedly: %v", result.err)
		}
	}
	if granted != 1 || blocked != workers-1 {
		t.Fatalf("concurrent push dispatch: granted=%d blocked=%d", granted, blocked)
	}
	var reservationCount, dispatchAuditCount int
	if err := store.db.QueryRow(`SELECT count(*) FROM notification_push_dispatches WHERE delivery_id=$1::uuid`, first.DeliveryID).Scan(&reservationCount); err != nil || reservationCount != 1 {
		t.Fatalf("expected exactly one durable reservation: count=%d err=%v", reservationCount, err)
	}
	if err := store.db.QueryRow(`SELECT count(*) FROM audit_records WHERE correlation_id=$1 AND action='NOTIFICATION_PUSH_DISPATCH_DECISION'`, first.IdempotencyKey).Scan(&dispatchAuditCount); err != nil || dispatchAuditCount != workers {
		t.Fatalf("all dispatch decisions must be audited: count=%d err=%v", dispatchAuditCount, err)
	}
	// A stale duplicate plan, despite still saying PENDING, cannot re-dispatch.
	staleAuditID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	if envelope, err := dispatcher.Reserve(context.Background(), first, staleAuditID, "system:notification"); !errors.Is(err, notification.ErrPushDispatchAlreadyReserved) || envelope.DeliveryID != "" {
		t.Fatalf("stale plan re-dispatched: %#v err=%v", envelope, err)
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
	// Every run of the existing PostgreSQL notification proof also exercises
	// scheduling, expiration and replay, rather than merely a unit mock.
	proveCanonicalNotificationDeliveryWindow(t, store, bookingID, providerID)
}

func proveCanonicalNotificationDeliveryWindow(t *testing.T, store *Checker, bookingID, providerID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	policy, err := notification.NewDeliveryPolicy("channel-policy-window-v1", []notification.Channel{notification.ChannelPush}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	service, err := notification.NewTransportService(store)
	if err != nil {
		t.Fatal(err)
	}
	insertIntent := func(notBefore, expiresAt, createdAt time.Time) notification.Intent {
		t.Helper()
		id, err := persistentid.New()
		if err != nil {
			t.Fatal(err)
		}
		key := "booking:" + bookingID + ":window:" + id
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO notification_intents
			 (id,booking_id,purpose,idempotency_key,transactional,data_class,
			  channel_policy_version,created_at,not_before,expires_at)
			VALUES ($1::uuid,$2::uuid,'BOOKING_CONFIRMATION',$3,true,'SENSITIVE',
			        'channel-policy-window-v1',$4,$5,$6)
		`, id, bookingID, key, createdAt, notBefore, expiresAt); err != nil {
			t.Fatal(err)
		}
		return notification.Intent{
			ID: id, BookingID: bookingID, Purpose: "BOOKING_CONFIRMATION",
			IdempotencyKey: key, Transactional: true,
			DataClass: "SENSITIVE", CreatedAt: createdAt,
		}
	}
	createRequest := func(endpoint string) notification.DeliveryRequest {
		t.Helper()
		deliveryID, err := persistentid.New()
		if err != nil {
			t.Fatal(err)
		}
		auditID, err := persistentid.New()
		if err != nil {
			t.Fatal(err)
		}
		return notification.DeliveryRequest{
			DeliveryID: deliveryID, AuditID: auditID, ActorID: "system:notification",
			Channel: notification.ChannelPush, EndpointRef: endpoint,
			ProviderInstanceID: providerID, OccurredAt: now,
		}
	}
	future := insertIntent(now.Add(time.Hour), now.Add(2*time.Hour), now)
	expired := insertIntent(now.Add(-2*time.Hour), now.Add(-time.Hour), now.Add(-3*time.Hour))
	futureRequest := createRequest("device/future-" + future.ID)
	expiredRequest := createRequest("device/expired-" + expired.ID)
	for _, tc := range []struct {
		intent  notification.Intent
		request notification.DeliveryRequest
		want    error
	}{
		{future, futureRequest, notification.ErrDeliveryNotBefore},
		{expired, expiredRequest, notification.ErrDeliveryExpired},
	} {
		if plan, duplicate, err := service.Claim(ctx, tc.intent, tc.request, policy); plan.DeliveryID != "" || duplicate || !errors.Is(err, tc.want) {
			t.Fatalf("timed notification accepted: plan=%#v duplicate=%v err=%v want=%v", plan, duplicate, err, tc.want)
		}
		var count int
		if err := store.db.QueryRowContext(ctx,
			"SELECT count(*) FROM notification_deliveries WHERE intent_id=$1::uuid",
			tc.intent.ID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("out-of-window intent generated delivery: count=%d err=%v", count, err)
		}
		key, keyErr := notification.DeliveryIdempotencyKey(tc.intent, tc.request.Channel, tc.request.EndpointRef)
		if keyErr != nil {
			t.Fatal(keyErr)
		}
		reason := notification.ReasonDeliveryNotBefore
		if errors.Is(tc.want, notification.ErrDeliveryExpired) {
			reason = notification.ReasonDeliveryExpired
		}
		if err := store.db.QueryRowContext(ctx,
			`SELECT count(*) FROM audit_records
			 WHERE correlation_id=$1 AND action='NOTIFICATION_DELIVERY_DECISION' AND reason=$2`,
			key, reason).Scan(&count); err != nil || count != 1 {
			t.Fatalf("delivery-window denial missing durable reason=%s audit: count=%d err=%v", reason, count, err)
		}
	}
	// An already persisted delivery is safe to replay after the intent expires.
	// It is not a new send, and the canonical idempotency owner must return it.
	key, err := notification.DeliveryIdempotencyKey(expired, expiredRequest.Channel, expiredRequest.EndpointRef)
	if err != nil {
		t.Fatal(err)
	}
	claimedAt := now.Add(-90 * time.Minute)
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO notification_deliveries
		 (id,intent_id,channel,endpoint_ref,provider_instance_id,
		  delivery_idempotency_key,state,created_at,updated_at)
		VALUES ($1::uuid,$2::uuid,'PUSH',$3,$4::uuid,$5,'PENDING',$6,$6)
	`, expiredRequest.DeliveryID, expired.ID, expiredRequest.EndpointRef,
		providerID, key, claimedAt); err != nil {
		t.Fatal(err)
	}
	replay := createRequest(expiredRequest.EndpointRef)
	result, duplicate, err := service.Claim(ctx, expired, replay, policy)
	if err != nil || !duplicate || result.DeliveryID != expiredRequest.DeliveryID {
		t.Fatalf("expired idempotent replay must recover existing row: %#v duplicate=%v err=%v", result, duplicate, err)
	}
	var count int
	if err := store.db.QueryRowContext(ctx,
		"SELECT count(*) FROM notification_deliveries WHERE intent_id=$1::uuid",
		expired.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("expired replay duplicated delivery: count=%d err=%v", count, err)
	}
}
