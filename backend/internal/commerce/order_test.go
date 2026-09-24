package commerce

import (
	"testing"
	"time"
)

func validOrderSnapshot() OrderSnapshot {
	return OrderSnapshot{
		ID:                      "order-1",
		BookingID:               "booking-1",
		OfferRef:                "offer-1",
		PriceSourceRef:          "catalog-price-1",
		AmountMinor:             10000,
		Currency:                "rub",
		CommissionMinor:         1000,
		PricingPolicyVersion:    "pricing-v1",
		CommissionPolicyVersion: "commission-v1",
		LegalSnapshotRef:        "legal-snapshot-1",
		SellerRef:               "identity/specialist-1",
		CommercialOwnerRef:      "identity/specialist-1",
		PaymentRecipientRef:     "identity/specialist-1",
		PlatformRole:            "MARKETPLACE_INTERMEDIARY",
		FiscalResponsibilityRef: "identity/specialist-1",
		RefundResponsibilityRef: "identity/specialist-1",
		PayoutBeneficiaryRef:    "identity/specialist-1",
		CapturedAt:              time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC),
	}
}

func TestOrderSnapshotCapturesEconomicAndLegalTruth(t *testing.T) {
	snapshot, err := NewOrderSnapshot(validOrderSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Currency != "RUB" {
		t.Fatalf("currency = %s", snapshot.Currency)
	}
	if snapshot.PricingPolicyVersion != "pricing-v1" ||
		snapshot.CommissionPolicyVersion != "commission-v1" ||
		snapshot.LegalSnapshotRef != "legal-snapshot-1" {
		t.Fatalf("snapshot lost policy/legal truth: %#v", snapshot)
	}
}

func TestOrderSnapshotRejectsSilentEconomicDefaults(t *testing.T) {
	for name, mutate := range map[string]func(*OrderSnapshot){
		"missing pricing policy":    func(s *OrderSnapshot) { s.PricingPolicyVersion = "" },
		"missing commission policy": func(s *OrderSnapshot) { s.CommissionPolicyVersion = "" },
		"missing legal snapshot":    func(s *OrderSnapshot) { s.LegalSnapshotRef = "" },
		"zero amount":               func(s *OrderSnapshot) { s.AmountMinor = 0 },
		"commission exceeds amount": func(s *OrderSnapshot) { s.CommissionMinor = s.AmountMinor + 1 },
		"invalid currency code":     func(s *OrderSnapshot) { s.Currency = "1$?" },
	} {
		t.Run(name, func(t *testing.T) {
			input := validOrderSnapshot()
			mutate(&input)
			if _, err := NewOrderSnapshot(input); err != ErrInvalidOrderSnapshot {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestOrderSnapshotIsValueCopy(t *testing.T) {
	input := validOrderSnapshot()
	snapshot, err := NewOrderSnapshot(input)
	if err != nil {
		t.Fatal(err)
	}
	input.AmountMinor = 1
	input.PricingPolicyVersion = "pricing-v999"

	if snapshot.AmountMinor != 10000 || snapshot.PricingPolicyVersion != "pricing-v1" {
		t.Fatalf("captured snapshot changed with source object: %#v", snapshot)
	}
}
