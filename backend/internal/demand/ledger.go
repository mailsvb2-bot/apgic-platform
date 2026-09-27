package demand

import "github.com/mailsvb2-bot/apgic-platform/backend/internal/ledger"

type LedgerStore interface {
	AppendLedgerEntry(entry ledger.Entry) error
	LedgerEntries() ([]ledger.Entry, error)
}

type ledgerState struct {
	entries []ledger.Entry
}

func (s *ledgerState) preview(entry ledger.Entry) ([]ledger.Entry, ledger.Entry, ledger.Reconciliation, error) {
	canonical, err := ledger.NewEntry(entry)
	if err != nil {
		return nil, ledger.Entry{}, ledger.Reconciliation{}, err
	}
	prospective := make([]ledger.Entry, 0, len(s.entries)+1)
	prospective = append(prospective, s.entries...)
	prospective = append(prospective, canonical)
	reconciliation, err := ledger.Reconcile(prospective)
	if err != nil {
		return nil, ledger.Entry{}, ledger.Reconciliation{}, err
	}
	return prospective, canonical, reconciliation, nil
}

func (s *ledgerState) commit(entries []ledger.Entry) {
	s.entries = append([]ledger.Entry(nil), entries...)
}

func (s *ledgerState) reconcile() (ledger.Reconciliation, error) {
	return ledger.Reconcile(s.entries)
}

func (s *Service) ledgerEntriesLocked() ([]ledger.Entry, error) {
	if s.ledgerStore != nil {
		return s.ledgerStore.LedgerEntries()
	}
	return append([]ledger.Entry(nil), s.ledgerState.entries...), nil
}

func (s *Service) previewLedgerLocked(entry ledger.Entry) ([]ledger.Entry, ledger.Entry, ledger.Reconciliation, error) {
	existing, err := s.ledgerEntriesLocked()
	if err != nil {
		return nil, ledger.Entry{}, ledger.Reconciliation{}, err
	}
	state := ledgerState{entries: existing}
	return state.preview(entry)
}

func (s *Service) commitLedgerEntryLocked(entry ledger.Entry, prospective []ledger.Entry) error {
	if s.ledgerStore != nil {
		return s.ledgerStore.AppendLedgerEntry(entry)
	}
	s.ledgerState.commit(prospective)
	return nil
}

func (s *Service) LedgerReconciliation() (ledger.Reconciliation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ledgerStore != nil {
		entries, err := s.ledgerStore.LedgerEntries()
		if err != nil {
			return ledger.Reconciliation{}, err
		}
		return ledger.Reconcile(entries)
	}
	return s.ledgerState.reconcile()
}
