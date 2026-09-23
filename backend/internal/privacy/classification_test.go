package privacy

import "testing"

func TestRawConsultationCannotFlowToGrowthWithoutPurposeConsent(t *testing.T) {
	if err := CanExportToGrowth(RawConsultation, false); err != ErrPurposeConsentRequired {
		t.Fatalf("unexpected result: %v", err)
	}
	if err := CanExportToGrowth(RawConsultation, true); err != nil {
		t.Fatalf("purpose-consented export should be allowed by this boundary: %v", err)
	}
}
