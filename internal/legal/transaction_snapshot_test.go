package legal

import "testing"

func TestTransactionSnapshotDoesNotInferEconomicRoles(t *testing.T) {
	snapshot := TransactionSnapshot{
		SellerOrServiceProviderID: "specialist-1",
		CommercialOwnerID:         "org-1",
		PaymentRecipientID:        "specialist-1",
		PlatformRole:              "MARKETPLACE_INTERMEDIARY",
		FiscalResponsibilityID:    "specialist-1",
		RefundResponsibilityID:    "specialist-1",
		PayoutBeneficiaryID:       "specialist-1",
		PolicyVersion:             "legal-v1",
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	snapshot.RefundResponsibilityID = ""
	if err := snapshot.Validate(); err == nil {
		t.Fatal("missing legal/economic role must fail closed")
	}
}
