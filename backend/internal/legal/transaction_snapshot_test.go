package legal

import "testing"

func TestTransactionSnapshotDoesNotInferEconomicRoles(t *testing.T) {
	s:=TransactionSnapshot{SellerOrServiceProviderID:"specialist-1",CommercialOwnerID:"org-1",PaymentRecipientID:"specialist-1",PlatformRole:"MARKETPLACE_INTERMEDIARY",FiscalResponsibilityID:"specialist-1",RefundResponsibilityID:"specialist-1",PayoutBeneficiaryID:"specialist-1",PolicyVersion:"legal-v1"}
	if err:=s.Validate(); err!=nil { t.Fatal(err) }
	s.RefundResponsibilityID=""
	if err:=s.Validate(); err==nil { t.Fatal("missing explicit role must fail closed") }
}
