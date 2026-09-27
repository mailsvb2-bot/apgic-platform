package demand

import (
	"errors"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/ledger"
)

type LedgerStore interface {
	AppendLedgerEntry(entry ledger.Entry) (ledger.Entry, error)
	LedgerEntries() ([]ledger.Entry, error)
}

type AtomicBookingLedgerStore interface {
	CommitBookingLedger(booked *booking.Booking, entry ledger.Entry) (ledger.Entry, error)
}

var ErrAtomicBookingLedgerRequired = errors.New("atomic booking-ledger store is required")

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
	if s.ledgerStore != nil {
		canonical, err := ledger.NewEntry(entry)
		if err != nil {
			return nil, ledger.Entry{}, ledger.Reconciliation{}, err
		}
		return nil, canonical, ledger.Reconciliation{}, nil
	}
	state := ledgerState{entries: append([]ledger.Entry(nil), s.ledgerState.entries...)}
	return state.preview(entry)
}

func (s *Service) commitBookingLedgerLocked(booked *booking.Booking, entry ledger.Entry, prospective []ledger.Entry) (ledger.Entry, error) {
	if s.ledgerStore != nil {
		atomicStore, ok := s.ledgerStore.(AtomicBookingLedgerStore)
		if s.journeyStore != nil && !ok {
			return ledger.Entry{}, ErrAtomicBookingLedgerRequired
		}
		if ok {
			return atomicStore.CommitBookingLedger(booked, entry)
		}
		return s.ledgerStore.AppendLedgerEntry(entry)
	}
	s.ledgerState.commit(prospective)
	return entry, nil
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
