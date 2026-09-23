package commerce

import "testing"

func TestCommercialOwnerAndRevenueBeneficiaryAreExplicit(t *testing.T) {
	got := Ownership{OwnerType: OwnerOrganization, OwnerID: "org-1"}
	if got.Validate() != ErrOwnershipIncomplete {
		t.Fatal("legal/revenue roles must not be inferred from owner id")
	}
	got.CommercialOwnerRef = "organization/org-1"
	got.RevenueBeneficiaryRef = "beneficiary/specialist-1"
	if err := got.Validate(); err != nil {
		t.Fatal(err)
	}
}
