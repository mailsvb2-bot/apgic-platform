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
	PriceSourceRef          string
	AmountMinor             int64
	Currency                string
	CommissionMinor         int64
	PricingPolicyVersion    string
	CommissionPolicyVersion string
	LegalSnapshotRef        string
	ProductID               string
	OrganizationDirectionID string
	ProductOwnerRef         string
	AuthorRefs              []string
	SellerRef               string
	CommercialOwnerRef      string
	PaymentRecipientRef     string
	PlatformRole            string
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
		snapshot.ProductID,
		snapshot.OrganizationDirectionID,
		snapshot.ProductOwnerRef,
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
	if len(snapshot.AuthorRefs) == 0 {
		return OrderSnapshot{}, ErrInvalidOrderSnapshot
	}
	seenAuthors := make(map[string]struct{}, len(snapshot.AuthorRefs))
	for _, author := range snapshot.AuthorRefs {
		author = strings.TrimSpace(author)
		if author == "" {
			return OrderSnapshot{}, ErrInvalidOrderSnapshot
		}
		if _, exists := seenAuthors[author]; exists {
			return OrderSnapshot{}, ErrInvalidOrderSnapshot
		}
		seenAuthors[author] = struct{}{}
	}
	snapshot.AuthorRefs = append([]string(nil), snapshot.AuthorRefs...)
	if snapshot.AmountMinor <= 0 ||
		snapshot.CommissionMinor < 0 ||
		snapshot.CommissionMinor > snapshot.AmountMinor ||
		!validCurrencyCode(snapshot.Currency) ||
		snapshot.CapturedAt.IsZero() {
		return OrderSnapshot{}, ErrInvalidOrderSnapshot
	}
	return snapshot, nil
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
