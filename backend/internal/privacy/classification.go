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

var ErrPurposeConsentRequired = errors.New("purpose-specific consent required")

func CanExportToGrowth(classification Classification, purposeConsent bool) error {
	switch classification {
	case RawConsultation, RawPersona:
		if !purposeConsent { return ErrPurposeConsentRequired }
	}
	return nil
}
