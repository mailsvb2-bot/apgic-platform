package commerce

import (
	"errors"
	"strings"
	"time"
)

type StoreLifecycleEventType string

const (
	StoreEventRenewal      StoreLifecycleEventType = "RENEWAL"
	StoreEventRefund       StoreLifecycleEventType = "REFUND"
	StoreEventRevocation   StoreLifecycleEventType = "REVOCATION"
	StoreEventChargeback   StoreLifecycleEventType = "CHARGEBACK"
	StoreEventGraceStarted StoreLifecycleEventType = "GRACE_STARTED"
	StoreEventHoldStarted  StoreLifecycleEventType = "HOLD_STARTED"
	StoreEventExpired      StoreLifecycleEventType = "EXPIRED"
)

type SubscriptionState string

const (
	SubscriptionActive  SubscriptionState = "ACTIVE"
	SubscriptionGrace   SubscriptionState = "GRACE"
	SubscriptionHold    SubscriptionState = "HOLD"
	SubscriptionExpired SubscriptionState = "EXPIRED"
	SubscriptionRevoked SubscriptionState = "REVOKED"
)

type EntitlementEffect string

const (
	EntitlementActivate EntitlementEffect = "ACTIVATE"
	EntitlementKeep     EntitlementEffect = "KEEP"
	EntitlementExpire   EntitlementEffect = "EXPIRE"
	EntitlementRevoke   EntitlementEffect = "REVOKE"
)

type LedgerEffect string

const (
	LedgerEffectNone       LedgerEffect = "NONE"
	LedgerEffectRenewal    LedgerEffect = "RENEWAL_CAPTURE"
	LedgerEffectRefund     LedgerEffect = "REFUND"
	LedgerEffectChargeback LedgerEffect = "CHARGEBACK"
)

var ErrInvalidStoreLifecycleEvent = errors.New("invalid store lifecycle event")

type StoreLifecycleEvent struct {
	EventID               string
	ProviderEventID       string
	ProviderInstanceID    string
	ExternalTransactionID string
	SubscriptionRef       string
	Sequence              int64
	Type                  StoreLifecycleEventType
	AmountMinor           int64
	Currency              string
	ProviderEvidenceRef   string
	LedgerEvidenceRef     string
	OccurredAt            time.Time
}

type StoreLifecycleEffect struct {
	SubscriptionState SubscriptionState
	EntitlementEffect EntitlementEffect
	LedgerEffect      LedgerEffect
}

type StoreLifecycleProjection struct {
	ProviderInstanceID string
	SubscriptionRef    string
	LastSequence       int64
	LastProviderEventID string
	State              SubscriptionState
	AppliedEventCount  int64
}

type StoreLifecycleDecision struct {
	Applied    bool
	Duplicate  bool
	Stale      bool
	Projection StoreLifecycleProjection
	Effect     StoreLifecycleEffect
	ReasonCode string
}

func ReconcileStoreLifecycle(
	current StoreLifecycleProjection,
	event StoreLifecycleEvent,
	seenProviderEventIDs map[string]struct{},
) (StoreLifecycleDecision, error) {
	if err := validateStoreLifecycleEvent(event); err != nil {
		return StoreLifecycleDecision{}, err
	}
	if current.ProviderInstanceID != "" &&
		(current.ProviderInstanceID != event.ProviderInstanceID ||
			current.SubscriptionRef != event.SubscriptionRef) {
		return StoreLifecycleDecision{}, ErrInvalidStoreLifecycleEvent
	}
	if _, exists := seenProviderEventIDs[event.ProviderEventID]; exists {
		return StoreLifecycleDecision{
			Duplicate: true,
			Projection: current,
			ReasonCode: "STORE_EVENT_DUPLICATE",
		}, nil
	}
	if current.LastSequence > 0 && event.Sequence <= current.LastSequence {
		return StoreLifecycleDecision{
			Stale: true,
			Projection: current,
			ReasonCode: "STORE_EVENT_STALE",
		}, nil
	}

	effect, err := storeLifecycleEffect(event.Type)
	if err != nil {
		return StoreLifecycleDecision{}, err
	}
	next := current
	if next.ProviderInstanceID == "" {
		next.ProviderInstanceID = event.ProviderInstanceID
		next.SubscriptionRef = event.SubscriptionRef
	}
	next.LastSequence = event.Sequence
	next.LastProviderEventID = event.ProviderEventID
	next.State = effect.SubscriptionState
	next.AppliedEventCount++

	return StoreLifecycleDecision{
		Applied: true,
		Projection: next,
		Effect: effect,
		ReasonCode: "STORE_EVENT_APPLIED",
	}, nil
}

func validateStoreLifecycleEvent(event StoreLifecycleEvent) error {
	event.Currency = strings.ToUpper(strings.TrimSpace(event.Currency))
	if strings.TrimSpace(event.EventID) == "" ||
		strings.TrimSpace(event.ProviderEventID) == "" ||
		strings.TrimSpace(event.ProviderInstanceID) == "" ||
		strings.TrimSpace(event.ExternalTransactionID) == "" ||
		strings.TrimSpace(event.SubscriptionRef) == "" ||
		event.Sequence <= 0 ||
		strings.TrimSpace(event.ProviderEvidenceRef) == "" ||
		event.OccurredAt.IsZero() {
		return ErrInvalidStoreLifecycleEvent
	}
	switch event.Type {
	case StoreEventRenewal, StoreEventRefund, StoreEventChargeback:
		if event.AmountMinor <= 0 ||
			len(event.Currency) != 3 ||
			strings.TrimSpace(event.LedgerEvidenceRef) == "" {
			return ErrInvalidStoreLifecycleEvent
		}
	case StoreEventRevocation, StoreEventGraceStarted, StoreEventHoldStarted, StoreEventExpired:
		if event.AmountMinor != 0 || event.Currency != "" || event.LedgerEvidenceRef != "" {
			return ErrInvalidStoreLifecycleEvent
		}
	default:
		return ErrInvalidStoreLifecycleEvent
	}
	return nil
}

func storeLifecycleEffect(kind StoreLifecycleEventType) (StoreLifecycleEffect, error) {
	switch kind {
	case StoreEventRenewal:
		return StoreLifecycleEffect{
			SubscriptionState: SubscriptionActive,
			EntitlementEffect: EntitlementActivate,
			LedgerEffect: LedgerEffectRenewal,
		}, nil
	case StoreEventRefund:
		return StoreLifecycleEffect{
			SubscriptionState: SubscriptionRevoked,
			EntitlementEffect: EntitlementRevoke,
			LedgerEffect: LedgerEffectRefund,
		}, nil
	case StoreEventChargeback:
		return StoreLifecycleEffect{
			SubscriptionState: SubscriptionRevoked,
			EntitlementEffect: EntitlementRevoke,
			LedgerEffect: LedgerEffectChargeback,
		}, nil
	case StoreEventRevocation:
		return StoreLifecycleEffect{
			SubscriptionState: SubscriptionRevoked,
			EntitlementEffect: EntitlementRevoke,
			LedgerEffect: LedgerEffectNone,
		}, nil
	case StoreEventGraceStarted:
		return StoreLifecycleEffect{
			SubscriptionState: SubscriptionGrace,
			EntitlementEffect: EntitlementKeep,
			LedgerEffect: LedgerEffectNone,
		}, nil
	case StoreEventHoldStarted:
		return StoreLifecycleEffect{
			SubscriptionState: SubscriptionHold,
			EntitlementEffect: EntitlementKeep,
			LedgerEffect: LedgerEffectNone,
		}, nil
	case StoreEventExpired:
		return StoreLifecycleEffect{
			SubscriptionState: SubscriptionExpired,
			EntitlementEffect: EntitlementExpire,
			LedgerEffect: LedgerEffectNone,
		}, nil
	default:
		return StoreLifecycleEffect{}, ErrInvalidStoreLifecycleEvent
	}
}
