package privacy

import "testing"

func TestSensitiveAndRawContentCannotFlowToGrowthWithoutPurposeConsent(t *testing.T) {
	for _, classification := range []Classification{Sensitive, RawConsultation, RawPersona} {
		if err := CanExportToGrowth(classification, false); err != ErrPurposeConsentRequired {
			t.Fatalf("%s: unexpected result: %v", classification, err)
		}
		if err := CanExportToGrowth(classification, true); err != nil {
			t.Fatalf("%s: consented export rejected: %v", classification, err)
		}
	}
}

func TestNonSensitiveGrowthProjectionDoesNotRequirePurposeConsent(t *testing.T) {
	for _, classification := range []Classification{Public, Internal} {
		if err := CanExportToGrowth(classification, false); err != nil {
			t.Fatalf("%s: minimal non-sensitive projection unexpectedly rejected: %v", classification, err)
		}
	}
}
