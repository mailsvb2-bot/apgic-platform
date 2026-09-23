package ledger

import (
	"errors"
	"strings"
	"time"
)

var ErrInvalidEntry = errors.New("ledger entry requires id, account refs, currency, non-zero amount, provider evidence and correlation id")

type Entry struct {
	ID                  string
	DebitAccountRef     string
	CreditAccountRef    string
	AmountMinor         int64
	Currency            string
	ProviderEvidenceRef string
	EconomicEventRef    string
	CorrelationID       string
	OccurredAt          time.Time
}

func NewEntry(e Entry) (Entry, error) {
	e.Currency = strings.ToUpper(e.Currency)
	if e.ID == "" || e.DebitAccountRef == "" || e.CreditAccountRef == "" ||
		e.AmountMinor <= 0 || len(e.Currency) != 3 || e.ProviderEvidenceRef == "" ||
		e.CorrelationID == "" {
		return Entry{}, ErrInvalidEntry
	}
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now().UTC()
	}
	return e, nil
}
