package commerce

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestCreditsAndBudgetsHaveNoCurrencyOrCashBalanceFields(t *testing.T) {
	typ := reflect.TypeOf(NonCashEntry{})
	for _, forbidden := range []string{"Currency", "AmountMinor", "CashBalance", "WalletBalance", "RecipientAccount"} {
		if _, ok := typ.FieldByName(forbidden); ok {
			t.Fatalf("non-cash entitlement must not contain monetary field %s", forbidden)
		}
	}
}

func TestCashOutAndPeerTransferAreForbidden(t *testing.T) {
	for _, operation := range []string{"CASH_OUT", "WITHDRAW", "TRANSFER", "P2P_TRANSFER"} {
		if !errors.Is(ValidateNonCashOperation(operation), ErrCashOperationForbidden) {
			t.Fatalf("%s must be forbidden for non-cash units", operation)
		}
	}
}

func TestNonCashSpendCannotExceedGrantedUnits(t *testing.T) {
	now := time.Now().UTC()
	entries := []NonCashEntry{
		{
			ID: "entry-1", AccountRef: "identity/1", UnitKind: UnitCredits,
			EventKind: NonCashGrant, Units: 100, SourceRef: "campaign/1",
			PolicyVersion: "credits-v1", OccurredAt: now,
		},
		{
			ID: "entry-2", AccountRef: "identity/1", UnitKind: UnitCredits,
			EventKind: NonCashSpend, Units: 120, SourceRef: "order/1",
			PolicyVersion: "credits-v1", OccurredAt: now.Add(time.Second),
		},
	}
	if _, err := Balance(entries); !errors.Is(err, ErrInsufficientUnits) {
		t.Fatalf("overspend must fail closed, got %v", err)
	}
}
