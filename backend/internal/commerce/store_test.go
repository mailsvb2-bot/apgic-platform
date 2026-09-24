package commerce

import (
	"errors"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/payments"
)

func TestStorePolicyReturnsExactlyOneConfiguredRail(t *testing.T) {
	policy := StorePolicySnapshot{
		Version: "store-policy-v1",
		EffectiveAt: time.Now().UTC(),
		Rules: []StorePolicyRule{
			{
				ProductType: "SUBSCRIPTION",
				Surface: SurfaceIOS,
				Store: "STORE_A",
				Storefront: "RU",
				Jurisdiction: "RU",
				Enabled: true,
				Rail: payments.RailStoreBilling,
				ReasonCode: "STORE_BILLING_REQUIRED",
			},
		},
	}
	decision, err := policy.Decide(StoreCommerceContext{
		ProductType: "SUBSCRIPTION",
		Surface: SurfaceIOS,
		Store: "STORE_A",
		Storefront: "RU",
		Jurisdiction: "RU",
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Outcome != StoreCommerceAllowed || decision.Rail != payments.RailStoreBilling {
		t.Fatalf("unexpected store decision: %#v", decision)
	}
}

func TestStorePolicyFailsClosedWhenCombinationMissing(t *testing.T) {
	policy := StorePolicySnapshot{
		Version: "store-policy-v1",
		EffectiveAt: time.Now().UTC(),
		Rules: []StorePolicyRule{
			{
				ProductType: "SUBSCRIPTION",
				Surface: SurfaceAndroid,
				Store: "STORE_B",
				Storefront: "RU",
				Jurisdiction: "RU",
				Enabled: true,
				Rail: payments.RailStoreBilling,
				ReasonCode: "STORE_BILLING_REQUIRED",
			},
		},
	}
	decision, err := policy.Decide(StoreCommerceContext{
		ProductType: "SUBSCRIPTION",
		Surface: SurfaceIOS,
		Store: "STORE_A",
		Storefront: "RU",
		Jurisdiction: "RU",
	})
	if !errors.Is(err, ErrStorePolicyNoMatch) {
		t.Fatalf("unknown store path must fail closed, got %v", err)
	}
	if decision.Outcome != StoreCommerceDisabled || decision.Rail != payments.RailPurchaseDisabled {
		t.Fatalf("unknown store path must be PURCHASE_DISABLED: %#v", decision)
	}
}

func TestStorePolicyRejectsAmbiguousRules(t *testing.T) {
	rule := StorePolicyRule{
		ProductType: "ONE_TIME",
		Surface: SurfaceIOS,
		Store: "STORE_A",
		Storefront: "US",
		Jurisdiction: "US",
		Enabled: true,
		Rail: payments.RailStoreBilling,
		ReasonCode: "STORE_BILLING_ALLOWED",
	}
	policy := StorePolicySnapshot{
		Version: "store-policy-v1",
		EffectiveAt: time.Now().UTC(),
		Rules: []StorePolicyRule{rule, rule},
	}
	_, err := policy.Decide(StoreCommerceContext{
		ProductType: "ONE_TIME",
		Surface: SurfaceIOS,
		Store: "STORE_A",
		Storefront: "US",
		Jurisdiction: "US",
	})
	if !errors.Is(err, ErrStorePolicyAmbiguous) {
		t.Fatalf("ambiguous store policy must fail closed, got %v", err)
	}
}

func TestVerifiedStorePurchaseRequiresCanonicalFinancialBasis(t *testing.T) {
	purchase := VerifiedStorePurchase{
		ID: "verification-1",
		IdentityID: "identity-1",
		OrderID: "order-1",
		PaymentAttemptID: "attempt-1",
		PaymentEffectID: "effect-1",
		LedgerEntryID: "ledger-1",
		ProviderInstanceID: "store-provider-1",
		ExternalTransactionID: "store-tx-1",
		ProductRef: "product/subscription-1",
		EntitlementKind: "SUBSCRIPTION",
		PolicyVersion: "store-policy-v1",
		ProviderEvidenceRef: "store-evidence/verified-1",
		VerifiedAt: time.Now().UTC(),
	}
	if err := purchase.Validate(); err != nil {
		t.Fatal(err)
	}
	purchase.LedgerEntryID = ""
	if !errors.Is(purchase.Validate(), ErrInvalidStorePurchase) {
		t.Fatal("store entitlement verification without ledger basis must be invalid")
	}
}
