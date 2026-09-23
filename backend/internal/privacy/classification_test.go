package privacy

import "testing"

func TestRawContentCannotFlowToGrowthWithoutPurposeConsent(t *testing.T) {
	for _, classification := range []Classification{RawConsultation, RawPersona} {
		if err := CanExportToGrowth(classification, false); err != ErrPurposeConsentRequired {
			t.Fatalf("%s: unexpected result: %v", classification, err)
		}
		if err := CanExportToGrowth(classification, true); err != nil {
			t.Fatalf("%s: consented export rejected: %v", classification, err)
		}
	}
}
