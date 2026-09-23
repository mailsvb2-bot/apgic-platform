package commerce

import "testing"

func TestCommercialOwnerAuthorAndRevenueBeneficiaryAreExplicit(t *testing.T) {
	ownership := Ownership{OwnerType: OwnerOrganization, OwnerID: "org-1"}
	if ownership.Validate() != ErrOwnershipIncomplete {
		t.Fatal("legal/revenue roles must not be inferred from owner id")
	}

	ownership.CommercialOwnerRef = "organization/org-1"
	ownership.RevenueBeneficiaryRef = "beneficiary/specialist-1"
	if ownership.Validate() != ErrOwnershipIncomplete {
		t.Fatal("author must be explicit")
	}

	ownership.AuthorRefs = []string{"identity/author-1"}
	if err := ownership.Validate(); err != nil {
		t.Fatal(err)
	}
}
