package commerce

import (
	"errors"
	"strings"
	"time"
)

type NonCashUnitKind string

const (
	UnitCredits            NonCashUnitKind = "CREDITS"
	UnitOrganizationBudget NonCashUnitKind = "ORGANIZATION_BUDGET"
	UnitGrowthBudget       NonCashUnitKind = "GROWTH_BUDGET"
)

type NonCashEventKind string

const (
	NonCashGrant  NonCashEventKind = "GRANT"
	NonCashSpend  NonCashEventKind = "SPEND"
	NonCashExpire NonCashEventKind = "EXPIRE"
)

var (
	ErrInvalidNonCashEntry = errors.New("invalid non-cash entitlement entry")
	ErrCashOperationForbidden = errors.New("cash-out and user-to-user money transfer are forbidden for non-cash entitlements")
	ErrInsufficientUnits = errors.New("insufficient non-cash entitlement units")
)

type NonCashEntry struct {
	ID             string
	AccountRef     string
	UnitKind       NonCashUnitKind
	EventKind      NonCashEventKind
	Units          int64
	SourceRef      string
	PolicyVersion  string
	OccurredAt     time.Time
}

func (e NonCashEntry) Validate() error {
	if strings.TrimSpace(e.ID) == "" ||
		strings.TrimSpace(e.AccountRef) == "" ||
		e.Units <= 0 ||
		strings.TrimSpace(e.SourceRef) == "" ||
		strings.TrimSpace(e.PolicyVersion) == "" ||
		e.OccurredAt.IsZero() {
		return ErrInvalidNonCashEntry
	}
	switch e.UnitKind {
	case UnitCredits, UnitOrganizationBudget, UnitGrowthBudget:
	default:
		return ErrInvalidNonCashEntry
	}
	switch e.EventKind {
	case NonCashGrant, NonCashSpend, NonCashExpire:
	default:
		return ErrInvalidNonCashEntry
	}
	return nil
}

func Balance(entries []NonCashEntry) (int64, error) {
	var balance int64
	for _, entry := range entries {
		if err := entry.Validate(); err != nil {
			return 0, err
		}
		switch entry.EventKind {
		case NonCashGrant:
			balance += entry.Units
		case NonCashSpend, NonCashExpire:
			balance -= entry.Units
		}
		if balance < 0 {
			return 0, ErrInsufficientUnits
		}
	}
	return balance, nil
}

func ValidateNonCashOperation(operation string) error {
	switch strings.ToUpper(strings.TrimSpace(operation)) {
	case "GRANT", "SPEND", "EXPIRE":
		return nil
	case "CASH_OUT", "WITHDRAW", "TRANSFER", "P2P_TRANSFER":
		return ErrCashOperationForbidden
	default:
		return ErrInvalidNonCashEntry
	}
}
