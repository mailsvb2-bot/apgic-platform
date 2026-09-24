package commerce

import (
	"errors"
	"strings"
	"time"
)

var ErrInvalidOrderSnapshot = errors.New("invalid immutable order snapshot")

type OrderSnapshot struct {
	ID                      string
	BookingID               string
	OfferRef                string
	PriceSourceRef           string
	AmountMinor             int64
	Currency                string
	CommissionMinor         int64
	PricingPolicyVersion    string
	CommissionPolicyVersion string
	LegalSnapshotRef        string
	SellerRef               string
	CommercialOwnerRef      string
	PaymentRecipientRef      string
	PlatformRole             string
	FiscalResponsibilityRef string
	RefundResponsibilityRef string
	PayoutBeneficiaryRef    string
	CapturedAt              time.Time
}

func NewOrderSnapshot(snapshot OrderSnapshot) (OrderSnapshot, error) {
	snapshot.Currency = strings.ToUpper(strings.TrimSpace(snapshot.Currency))
	required := []string{
		snapshot.ID,
		snapshot.BookingID,
		snapshot.OfferRef,
		snapshot.PriceSourceRef,
		snapshot.PricingPolicyVersion,
		snapshot.CommissionPolicyVersion,
		snapshot.LegalSnapshotRef,
		snapshot.SellerRef,
		snapshot.CommercialOwnerRef,
		snapshot.PaymentRecipientRef,
		snapshot.PlatformRole,
		snapshot.FiscalResponsibilityRef,
		snapshot.RefundResponsibilityRef,
		snapshot.PayoutBeneficiaryRef,
	}
	for _, value := range required {
		if strings.TrimSpace(value) == "" {
			return OrderSnapshot{}, ErrInvalidOrderSnapshot
		}
	}
	if snapshot.AmountMinor <= 0 ||
		snapshot.CommissionMinor < 0 ||
		snapshot.CommissionMinor > snapshot.AmountMinor ||
		len(snapshot.Currency) != 3 ||
		snapshot.CapturedAt.IsZero() {
		return OrderSnapshot{}, ErrInvalidOrderSnapshot
	}
	return snapshot, nil
}
