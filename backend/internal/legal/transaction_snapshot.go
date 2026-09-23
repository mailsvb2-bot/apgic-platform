package legal

import (
	"errors"
	"strings"
)

var ErrIncompleteTransactionSnapshot = errors.New("incomplete legal transaction snapshot")

type TransactionSnapshot struct {
	SellerOrServiceProviderID string
	CommercialOwnerID         string
	PaymentRecipientID        string
	PlatformRole              string
	FiscalResponsibilityID    string
	RefundResponsibilityID    string
	PayoutBeneficiaryID       string
	PolicyVersion             string
}

func (s TransactionSnapshot) Validate() error {
	values := []string{
		s.SellerOrServiceProviderID,
		s.CommercialOwnerID,
		s.PaymentRecipientID,
		s.PlatformRole,
		s.FiscalResponsibilityID,
		s.RefundResponsibilityID,
		s.PayoutBeneficiaryID,
		s.PolicyVersion,
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return ErrIncompleteTransactionSnapshot
		}
	}
	return nil
}
