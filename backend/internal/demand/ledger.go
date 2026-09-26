package demand

import "github.com/mailsvb2-bot/apgic-platform/backend/internal/ledger"

type ledgerState struct {
	entries []ledger.Entry
}

func (s *ledgerState) preview(entry ledger.Entry) ([]ledger.Entry, ledger.Reconciliation, error) {
	prospective := make([]ledger.Entry, 0, len(s.entries)+1)
	prospective = append(prospective, s.entries...)
	prospective = append(prospective, entry)

	reconciliation, err := ledger.Reconcile(prospective)
	if err != nil {
		return nil, ledger.Reconciliation{}, err
	}
	return prospective, reconciliation, nil
}

func (s *ledgerState) commit(entries []ledger.Entry) {
	s.entries = append([]ledger.Entry(nil), entries...)
}

func (s *ledgerState) reconcile() (ledger.Reconciliation, error) {
	return ledger.Reconcile(s.entries)
}

func (s *Service) LedgerReconciliation() (ledger.Reconciliation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ledgerState.reconcile()
}
