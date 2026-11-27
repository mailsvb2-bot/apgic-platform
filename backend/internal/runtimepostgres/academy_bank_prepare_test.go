package runtimepostgres

import (
	"errors"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

func TestAcademyBankPrepareRejectsUnsafeInputBeforeDB(t *testing.T) {
	var absent *Checker
	policy := AcademyBankLegalPolicy{
		SellerRef: "identity/seller", CommercialOwnerRef: "identity/seller",
		PaymentRecipientRef: "identity/seller", PlatformRole: "DIRECT_SELLER",
		FiscalResponsibilityRef: "identity/seller", RefundResponsibilityRef: "identity/seller",
		PayoutBeneficiaryRef: "identity/seller", LegalPolicyVersion: "approved-v1",
		PricingPolicyVersion: "approved-v1", CommissionPolicyVersion: "approved-v1",
		RoutingPolicyVersion: "approved-v1", ProviderConfigID: "00000000-0000-0000-0000-000000000003",
	}
	buyer := "00000000-0000-0000-0000-000000000001"
	course := "00000000-0000-0000-0000-000000000002"
	for _, tc := range []struct {
		name, buyer, course, key string
		mutate                   func(*AcademyBankLegalPolicy)
	}{
		{"missing principal", "", course, "valid_request_123", nil},
		{"foreign principal format", "attacker", course, "valid_request_123", nil},
		{"invalid course", buyer, "../../x", "valid_request_123", nil},
		{"missing idempotency", buyer, course, "", nil},
		{"short idempotency", buyer, course, "123", nil},
		{"unsafe idempotency", buyer, course, "unsafe/../path", nil},
		{"untrusted provider ID", buyer, course, "valid_request_123", func(p *AcademyBankLegalPolicy) { p.ProviderConfigID = "../provider" }},
		{"missing seller", buyer, course, "valid_request_123", func(p *AcademyBankLegalPolicy) { p.SellerRef = "" }},
		{"missing fiscal owner", buyer, course, "valid_request_123", func(p *AcademyBankLegalPolicy) { p.FiscalResponsibilityRef = "" }},
		{"blank routing policy", buyer, course, "valid_request_123", func(p *AcademyBankLegalPolicy) { p.RoutingPolicyVersion = "  " }},
		{"trimmed seller", buyer, course, "valid_request_123", func(p *AcademyBankLegalPolicy) { p.SellerRef = " identity/seller " }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := policy
			if tc.mutate != nil {
				tc.mutate(&candidate)
			}
			_, err := absent.AcademyPrepareBankAttempt(tc.buyer, tc.course, tc.key, candidate)
			if !errors.Is(err, academy.ErrInvalid) {
				t.Fatalf("invalid canonical bank checkout was admitted: %v", err)
			}
		})
	}
}
