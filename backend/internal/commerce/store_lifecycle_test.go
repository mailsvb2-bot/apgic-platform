package commerce

import (
	"errors"
	"testing"
	"time"
)

func storeEvent(sequence int64, kind StoreLifecycleEventType) StoreLifecycleEvent {
	event := StoreLifecycleEvent{
		EventID: "event-" + string(rune('a'+sequence)),
		ProviderEventID: "provider-event-" + string(rune('a'+sequence)),
		ProviderInstanceID: "store-provider-a",
		ExternalTransactionID: "store-transaction-1",
		SubscriptionRef: "subscription-1",
		Sequence: sequence,
		Type: kind,
		ProviderEvidenceRef: "provider-evidence/store-event",
		OccurredAt: time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC).Add(time.Duration(sequence) * time.Minute),
	}
	if kind == StoreEventRenewal || kind == StoreEventRefund || kind == StoreEventChargeback {
		event.AmountMinor = 10000
		event.Currency = "RUB"
		event.LedgerEvidenceRef = "ledger/store-event"
	}
	return event
}

func TestStoreLifecycleDuplicateAndOutOfOrderEventsDoNotRewriteProjection(t *testing.T) {
	seen := map[string]struct{}{}
	current := StoreLifecycleProjection{}

	first := storeEvent(1, StoreEventRenewal)
	result, err := ReconcileStoreLifecycle(current, first, seen)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Applied || result.Projection.State != SubscriptionActive || result.Projection.AppliedEventCount != 1 {
		t.Fatalf("renewal result=%#v", result)
	}
	current = result.Projection
	seen[first.ProviderEventID] = struct{}{}

	duplicate, err := ReconcileStoreLifecycle(current, first, seen)
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate.Duplicate || duplicate.Applied || duplicate.Projection != current {
		t.Fatalf("duplicate result=%#v", duplicate)
	}

	newer := storeEvent(3, StoreEventGraceStarted)
	result, err = ReconcileStoreLifecycle(current, newer, seen)
	if err != nil {
		t.Fatal(err)
	}
	if result.Projection.State != SubscriptionGrace || result.Projection.LastSequence != 3 {
		t.Fatalf("newer result=%#v", result)
	}
	current = result.Projection

	stale := storeEvent(2, StoreEventHoldStarted)
	result, err = ReconcileStoreLifecycle(current, stale, seen)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Stale || result.Applied || result.Projection != current {
		t.Fatalf("stale event rewrote projection: %#v", result)
	}
}

func TestFinancialStoreEventsRequireLedgerEvidence(t *testing.T) {
	event := storeEvent(1, StoreEventRefund)
	event.LedgerEvidenceRef = ""
	if _, err := ReconcileStoreLifecycle(StoreLifecycleProjection{}, event, map[string]struct{}{}); !errors.Is(err, ErrInvalidStoreLifecycleEvent) {
		t.Fatalf("refund without ledger evidence must fail, got %v", err)
	}

	event = storeEvent(1, StoreEventChargeback)
	event.AmountMinor = 0
	if _, err := ReconcileStoreLifecycle(StoreLifecycleProjection{}, event, map[string]struct{}{}); !errors.Is(err, ErrInvalidStoreLifecycleEvent) {
		t.Fatalf("chargeback without amount must fail, got %v", err)
	}
}

func TestStoreLifecycleMapsProviderEventsToExplicitCanonicalEffects(t *testing.T) {
	cases := []struct {
		kind        StoreLifecycleEventType
		state       SubscriptionState
		entitlement EntitlementEffect
		ledger      LedgerEffect
	}{
		{StoreEventRenewal, SubscriptionActive, EntitlementActivate, LedgerEffectRenewal},
		{StoreEventRefund, SubscriptionRevoked, EntitlementRevoke, LedgerEffectRefund},
		{StoreEventRevocation, SubscriptionRevoked, EntitlementRevoke, LedgerEffectNone},
		{StoreEventChargeback, SubscriptionRevoked, EntitlementRevoke, LedgerEffectChargeback},
		{StoreEventGraceStarted, SubscriptionGrace, EntitlementKeep, LedgerEffectNone},
		{StoreEventHoldStarted, SubscriptionHold, EntitlementKeep, LedgerEffectNone},
		{StoreEventExpired, SubscriptionExpired, EntitlementExpire, LedgerEffectNone},
	}
	for i, tc := range cases {
		event := storeEvent(int64(i+1), tc.kind)
		result, err := ReconcileStoreLifecycle(StoreLifecycleProjection{}, event, map[string]struct{}{})
		if err != nil {
			t.Fatalf("%s: %v", tc.kind, err)
		}
		if result.Effect.SubscriptionState != tc.state ||
			result.Effect.EntitlementEffect != tc.entitlement ||
			result.Effect.LedgerEffect != tc.ledger {
			t.Fatalf("%s: effect=%#v", tc.kind, result.Effect)
		}
	}
}

func TestStoreLifecycleCannotCrossProviderOrSubscription(t *testing.T) {
	current := StoreLifecycleProjection{
		ProviderInstanceID: "store-provider-a",
		SubscriptionRef: "subscription-1",
		LastSequence: 1,
		State: SubscriptionActive,
		AppliedEventCount: 1,
	}
	event := storeEvent(2, StoreEventRenewal)
	event.ProviderInstanceID = "store-provider-b"
	if _, err := ReconcileStoreLifecycle(current, event, map[string]struct{}{}); !errors.Is(err, ErrInvalidStoreLifecycleEvent) {
		t.Fatalf("cross-provider store lifecycle must fail, got %v", err)
	}
}
