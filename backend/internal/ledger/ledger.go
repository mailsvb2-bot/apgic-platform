package ledger

import (
	"errors"
	"sort"
	"strings"
	"time"
)

var (
	ErrInvalidEntry      = errors.New("ledger entry requires id, account refs, currency, non-zero amount, provider evidence and correlation id")
	ErrDuplicateEntryID  = errors.New("ledger reconciliation contains duplicate entry id")
	ErrReconcileOverflow = errors.New("ledger reconciliation exceeds int64 minor-unit range")
)

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

type AccountBalance struct {
	AccountRef  string
	Currency    string
	AmountMinor int64
}

type CurrencyTotals struct {
	Currency    string
	DebitMinor  int64
	CreditMinor int64
}

type Reconciliation struct {
	EntryCount     int
	Balances       []AccountBalance
	CurrencyTotals []CurrencyTotals
}

func NewEntry(e Entry) (Entry, error) {
	e.Currency = strings.ToUpper(e.Currency)
	if e.ID == "" || e.DebitAccountRef == "" || e.CreditAccountRef == "" ||
		e.AmountMinor <= 0 || !validCurrencyCode(e.Currency) || e.ProviderEvidenceRef == "" ||
		e.CorrelationID == "" {
		return Entry{}, ErrInvalidEntry
	}
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now().UTC()
	}
	return e, nil
}

// Reconcile deterministically replays append-only ledger entries into per-account,
// per-currency balances. It never treats the result as a custodial cash balance:
// provider evidence remains mandatory on every source entry.
func Reconcile(entries []Entry) (Reconciliation, error) {
	type balanceKey struct {
		accountRef string
		currency   string
	}
	type totals struct {
		debit  int64
		credit int64
	}

	seen := make(map[string]struct{}, len(entries))
	balances := make(map[balanceKey]int64)
	byCurrency := make(map[string]totals)

	for _, raw := range entries {
		entry, err := NewEntry(raw)
		if err != nil {
			return Reconciliation{}, err
		}
		if _, exists := seen[entry.ID]; exists {
			return Reconciliation{}, ErrDuplicateEntryID
		}
		seen[entry.ID] = struct{}{}

		debitKey := balanceKey{accountRef: entry.DebitAccountRef, currency: entry.Currency}
		creditKey := balanceKey{accountRef: entry.CreditAccountRef, currency: entry.Currency}

		nextDebit, ok := addInt64(balances[debitKey], -entry.AmountMinor)
		if !ok {
			return Reconciliation{}, ErrReconcileOverflow
		}
		balances[debitKey] = nextDebit

		nextCredit, ok := addInt64(balances[creditKey], entry.AmountMinor)
		if !ok {
			return Reconciliation{}, ErrReconcileOverflow
		}
		balances[creditKey] = nextCredit

		current := byCurrency[entry.Currency]
		current.debit, ok = addInt64(current.debit, entry.AmountMinor)
		if !ok {
			return Reconciliation{}, ErrReconcileOverflow
		}
		current.credit, ok = addInt64(current.credit, entry.AmountMinor)
		if !ok {
			return Reconciliation{}, ErrReconcileOverflow
		}
		byCurrency[entry.Currency] = current
	}

	result := Reconciliation{EntryCount: len(entries)}
	for key, amount := range balances {
		result.Balances = append(result.Balances, AccountBalance{
			AccountRef:  key.accountRef,
			Currency:    key.currency,
			AmountMinor: amount,
		})
	}
	sort.Slice(result.Balances, func(i, j int) bool {
		if result.Balances[i].Currency != result.Balances[j].Currency {
			return result.Balances[i].Currency < result.Balances[j].Currency
		}
		return result.Balances[i].AccountRef < result.Balances[j].AccountRef
	})

	for currency, total := range byCurrency {
		if total.debit != total.credit {
			return Reconciliation{}, ErrInvalidEntry
		}
		result.CurrencyTotals = append(result.CurrencyTotals, CurrencyTotals{
			Currency:    currency,
			DebitMinor:  total.debit,
			CreditMinor: total.credit,
		})
	}
	sort.Slice(result.CurrencyTotals, func(i, j int) bool {
		return result.CurrencyTotals[i].Currency < result.CurrencyTotals[j].Currency
	})

	return result, nil
}

func addInt64(left, right int64) (int64, bool) {
	sum := left + right
	if right > 0 && sum < left {
		return 0, false
	}
	if right < 0 && sum > left {
		return 0, false
	}
	return sum, true
}

func validCurrencyCode(code string) bool {
	if len(code) != 3 {
		return false
	}
	for i := 0; i < len(code); i++ {
		if code[i] < 'A' || code[i] > 'Z' {
			return false
		}
	}
	return true
}
