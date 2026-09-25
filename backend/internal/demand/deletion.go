package demand

import (
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/privacy"
)

const (
	deletionNotice = "Это удаление учётной записи, не деактивация. Профиль стирается. Запись учёта оплаты сохраняется: APGIC её не уничтожает и деньги не перемещает."
	ledgerReason   = "Запись учёта оплаты хранится по правилу хранения."
)

type AccountDeletion struct {
	ID                 string `json:"id"`
	IdentityID         string `json:"identity_id"`
	Source             string `json:"source"`
	State              string `json:"state"`
	Deactivation       bool   `json:"deactivation"`
	ProfileErased      bool   `json:"profile_erased"`
	LedgerRetained     bool   `json:"ledger_retained"`
	LedgerID           string `json:"ledger_id,omitempty"`
	LedgerReason       string `json:"ledger_reason"`
	ProviderRef        string `json:"provider_ref"`
	ProviderEvidence   string `json:"provider_evidence"`
	APGICDeletesLedger bool   `json:"apgic_deletes_ledger"`
	Idempotent         bool   `json:"idempotent"`
	Notice             string `json:"notice"`
}

func (s *Service) DeleteAccount(identityID, source string) (*AccountDeletion, error) {
	if strings.EqualFold(strings.TrimSpace(source), "DEACTIVATED") || strings.EqualFold(strings.TrimSpace(source), "DEACTIVATION") {
		return nil, ErrNotDeletion
	}
	if strings.TrimSpace(source) == "" {
		source = string(privacy.DeletionSourceWeb)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.owners[identityID]; !ok {
		return nil, ErrIdentityMismatch
	}
	if existing := s.deletions[identityID]; existing != nil {
		copyDeletion := *existing
		copyDeletion.Idempotent = true
		return &copyDeletion, nil
	}
	now := s.now().UTC()
	request, err := privacy.NewDeleteAccountRequest(newID("del-"), identityID, privacy.DeletionSource(source), now)
	if err != nil {
		return nil, ErrNotDeletion
	}
	if err := request.ReconfirmIdentity(now); err != nil {
		return nil, err
	}
	if err := request.ClassifyRetention([]privacy.RetentionItem{
		{DataClass: "PROFILE", Disposition: privacy.RetentionErase},
		{DataClass: "HELP_INTENT", Disposition: privacy.RetentionErase},
		{DataClass: "FINANCIAL_EVIDENCE", Disposition: privacy.RetentionRetain, Reason: ledgerReason},
	}, now); err != nil {
		return nil, err
	}
	providerRef := "profile-external"
	if err := request.BeginProviderErasure([]privacy.ProviderErasureJob{
		{ProviderRef: providerRef, State: privacy.ErasurePending},
	}, now); err != nil {
		return nil, err
	}
	evidenceRef := "provider-erasure:" + identityID
	if err := request.MarkProviderErased(providerRef, evidenceRef, now); err != nil {
		return nil, err
	}
	if err := request.Complete(now); err != nil {
		return nil, err
	}
	created := &AccountDeletion{
		ID:                 request.ID,
		IdentityID:         identityID,
		Source:             source,
		State:              string(request.State),
		Deactivation:       false,
		ProfileErased:      true,
		LedgerRetained:     true,
		LedgerID:           s.ledgerIDLocked(identityID),
		LedgerReason:       ledgerReason,
		ProviderRef:        providerRef,
		ProviderEvidence:   evidenceRef,
		APGICDeletesLedger: false,
		Notice:             deletionNotice,
	}
	if created.State != string(privacy.DeletionPartiallyRetainedReason) || created.APGICDeletesLedger {
		return nil, ErrNotDeletion
	}
	s.deletions[identityID] = created
	copyDeletion := *created
	return &copyDeletion, nil
}

func (s *Service) ledgerIDLocked(identityID string) string {
	for _, item := range s.evidence {
		booked := s.bookings[item.BookingID]
		if booked != nil && booked.ClientIdentityID == identityID && item.LedgerEntryID != "" {
			return item.LedgerEntryID
		}
	}
	return ""
}
