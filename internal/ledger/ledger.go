package ledger

import (
	"errors"
	"strings"
)

type Side string

const (
	Debit  Side = "DEBIT"
	Credit Side = "CREDIT"
)

var (
	ErrInvalidEntry = errors.New("invalid ledger entry")
	ErrUnbalanced   = errors.New("ledger batch is not balanced")
)

type Entry struct {
	ID          string
	EvidenceID  string
	Account     string
	Currency    string
	Side        Side
	AmountMinor int64
}

func (e Entry) Validate() error {
	if strings.TrimSpace(e.ID) == "" ||
		strings.TrimSpace(e.EvidenceID) == "" ||
		strings.TrimSpace(e.Account) == "" ||
		strings.TrimSpace(e.Currency) == "" ||
		e.AmountMinor <= 0 ||
		(e.Side != Debit && e.Side != Credit) {
		return ErrInvalidEntry
	}
	return nil
}

func ValidateBalanced(entries []Entry) error {
	type totals struct{ debit, credit int64 }
	perCurrency := make(map[string]totals)
	for _, entry := range entries {
		if err := entry.Validate(); err != nil {
			return err
		}
		value := perCurrency[entry.Currency]
		if entry.Side == Debit {
			value.debit += entry.AmountMinor
		} else {
			value.credit += entry.AmountMinor
		}
		perCurrency[entry.Currency] = value
	}
	if len(perCurrency) == 0 {
		return ErrUnbalanced
	}
	for _, value := range perCurrency {
		if value.debit != value.credit {
			return ErrUnbalanced
		}
	}
	return nil
}
