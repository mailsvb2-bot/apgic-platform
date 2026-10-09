package privacy

import "errors"

type Classification string

const (
	Public          Classification = "PUBLIC"
	Internal        Classification = "INTERNAL"
	Sensitive       Classification = "SENSITIVE"
	RawConsultation Classification = "RAW_CONSULTATION"
	RawPersona      Classification = "RAW_PERSONA"
)

var (
	ErrPurposeConsentRequired = errors.New("purpose-specific consent required")
	ErrClassificationUnknown  = errors.New("data classification is unknown")
)

func CanExportToGrowth(classification Classification, purposeConsent bool) error {
	switch classification {
	case Public, Internal:
		return nil
	case Sensitive, RawConsultation, RawPersona:
		if !purposeConsent {
			return ErrPurposeConsentRequired
		}
		return nil
	default:
		return ErrClassificationUnknown
	}
}
