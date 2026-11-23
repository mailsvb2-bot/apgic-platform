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
		ProductID:               "product-1",
		OrganizationDirectionID: "direction-1",
		ProductOwnerRef:         "organization/org-1",
		AuthorRefs:              []string{"identity/author-1"},
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
		"missing product":           func(s *OrderSnapshot) { s.ProductID = "" },
		"missing direction":         func(s *OrderSnapshot) { s.OrganizationDirectionID = "" },
		"missing product owner":     func(s *OrderSnapshot) { s.ProductOwnerRef = "" },
		"missing authors":           func(s *OrderSnapshot) { s.AuthorRefs = nil },
		"blank author":              func(s *OrderSnapshot) { s.AuthorRefs = []string{""} },
		"duplicate author":          func(s *OrderSnapshot) { s.AuthorRefs = []string{"identity/author-1", "identity/author-1"} },
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
	input.AuthorRefs[0] = "identity/mutated"

	if snapshot.AmountMinor != 10000 || snapshot.PricingPolicyVersion != "pricing-v1" || snapshot.AuthorRefs[0] != "identity/author-1" {
		t.Fatalf("captured snapshot changed with source object: %#v", snapshot)
	}
}

func TestOrderSnapshotDirectAcademyUsesSameCommerceOwnerWithoutBooking(t *testing.T) {
	s := validOrderSnapshot()
	s.BookingID = ""
	s.OrganizationDirectionID = "" // Identity-owned published product.
	s.DirectBuyerIdentityID = "22222222-2222-4222-8222-222222222222"
	s.DirectCheckoutIdempotencyKey = "academy_checkout_abc_1"
	s.OfferRef = "academy/course/33333333-3333-4333-8333-333333333333"
	s.PriceSourceRef = "academy/offer/revision/7"
	s.Currency = "RUB"
	got, err := NewOrderSnapshot(s)
	if err != nil {
		t.Fatal(err)
	}
	if got.BookingID != "" || got.DirectBuyerIdentityID != s.DirectBuyerIdentityID ||
		got.DirectCheckoutIdempotencyKey != s.DirectCheckoutIdempotencyKey {
		t.Fatalf("direct order lost durable customer identity: %#v", got)
	}
	for name, mutate := range map[string]func(*OrderSnapshot){
		"missing identity":    func(v *OrderSnapshot) { v.DirectBuyerIdentityID = "" },
		"missing idempotency": func(v *OrderSnapshot) { v.DirectCheckoutIdempotencyKey = "" },
		"invalid key":         func(v *OrderSnapshot) { v.DirectCheckoutIdempotencyKey = "../\nother" },
		"wrong offer":         func(v *OrderSnapshot) { v.OfferRef = "book/offer" },
		"wrong price source":  func(v *OrderSnapshot) { v.PriceSourceRef = "client/price/1" },
		"wrong currency":      func(v *OrderSnapshot) { v.Currency = "USD" },
	} {
		t.Run(name, func(t *testing.T) {
			v := s
			mutate(&v)
			if _, err := NewOrderSnapshot(v); err != ErrInvalidOrderSnapshot {
				t.Fatalf("unsafe direct order accepted: %v", err)
			}
		})
	}
}

func TestOrderSnapshotBookedPathCannotSmuggleDirectBuyer(t *testing.T) {
	for _, mutate := range []func(*OrderSnapshot){
		func(v *OrderSnapshot) { v.DirectBuyerIdentityID = "22222222-2222-4222-8222-222222222222" },
		func(v *OrderSnapshot) { v.DirectCheckoutIdempotencyKey = "another_checkout_1" },
	} {
		v := validOrderSnapshot()
		mutate(&v)
		if _, err := NewOrderSnapshot(v); err != ErrInvalidOrderSnapshot {
			t.Fatalf("booked order with direct buyer metadata accepted: %v", err)
		}
	}
}
