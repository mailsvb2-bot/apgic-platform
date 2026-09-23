package commerce

import "testing"

func TestOwnershipRequiresExplicitEconomicRoles(t *testing.T) {
	ownership := Ownership{
		OwnerID:              "identity-1",
		CommercialOwnerID:    "org-1",
		RevenueBeneficiaryID: "identity-1",
	}
	if err := ownership.Validate(); err != nil {
		t.Fatal(err)
	}
	ownership.CommercialOwnerID = ""
	if err := ownership.Validate(); err == nil {
		t.Fatal("missing commercial owner must be rejected")
	}
}
