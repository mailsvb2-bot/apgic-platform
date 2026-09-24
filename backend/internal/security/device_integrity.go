package security

import (
	"errors"
	"strings"
	"time"
)

type IntegrityVerdict string

const (
	IntegrityValid       IntegrityVerdict = "VALID"
	IntegrityNegative    IntegrityVerdict = "NEGATIVE"
	IntegrityUnsupported IntegrityVerdict = "UNSUPPORTED"
	IntegrityUnavailable IntegrityVerdict = "UNAVAILABLE"
)

type IntegrityAction string

const (
	IntegrityAllow  IntegrityAction = "ALLOW"
	IntegrityStepUp IntegrityAction = "STEP_UP"
	IntegrityReview IntegrityAction = "REVIEW"
)

var ErrInvalidIntegrityEvidence = errors.New("invalid device integrity evidence")

type DeviceIntegrityEvidence struct {
	ID                  string
	IdentityID          string
	InstallationID      string
	ProviderKind        string
	ProviderEvidenceRef string
	Verdict             IntegrityVerdict
	ServerVerified      bool
	ObservedAt          time.Time
}

func (e DeviceIntegrityEvidence) Validate() error {
	if strings.TrimSpace(e.ID) == "" ||
		strings.TrimSpace(e.IdentityID) == "" ||
		strings.TrimSpace(e.InstallationID) == "" ||
		e.ObservedAt.IsZero() {
		return ErrInvalidIntegrityEvidence
	}
	switch e.Verdict {
	case IntegrityUnsupported:
		if e.ServerVerified || strings.TrimSpace(e.ProviderEvidenceRef) != "" {
			return ErrInvalidIntegrityEvidence
		}
	case IntegrityValid, IntegrityNegative:
		if !e.ServerVerified ||
			strings.TrimSpace(e.ProviderKind) == "" ||
			strings.TrimSpace(e.ProviderEvidenceRef) == "" {
			return ErrInvalidIntegrityEvidence
		}
	case IntegrityUnavailable:
		if strings.TrimSpace(e.ProviderKind) == "" {
			return ErrInvalidIntegrityEvidence
		}
	default:
		return ErrInvalidIntegrityEvidence
	}
	return nil
}

type IntegrityRiskDecision struct {
	Action        IntegrityAction
	ReasonCode    string
	PolicyVersion string
	AppealPath    string
}

func EvaluateIntegrity(e DeviceIntegrityEvidence, policyVersion, appealPath string) (IntegrityRiskDecision, error) {
	if err := e.Validate(); err != nil ||
		strings.TrimSpace(policyVersion) == "" ||
		strings.TrimSpace(appealPath) == "" {
		return IntegrityRiskDecision{}, ErrInvalidIntegrityEvidence
	}
	switch e.Verdict {
	case IntegrityValid:
		return IntegrityRiskDecision{
			Action:        IntegrityAllow,
			ReasonCode:    "DEVICE_INTEGRITY_VALID",
			PolicyVersion: policyVersion,
			AppealPath:    appealPath,
		}, nil
	case IntegrityNegative:
		return IntegrityRiskDecision{
			Action:        IntegrityStepUp,
			ReasonCode:    "DEVICE_INTEGRITY_NEGATIVE_STEP_UP",
			PolicyVersion: policyVersion,
			AppealPath:    appealPath,
		}, nil
	case IntegrityUnsupported:
		return IntegrityRiskDecision{
			Action:        IntegrityReview,
			ReasonCode:    "DEVICE_INTEGRITY_UNSUPPORTED",
			PolicyVersion: policyVersion,
			AppealPath:    appealPath,
		}, nil
	case IntegrityUnavailable:
		return IntegrityRiskDecision{
			Action:        IntegrityReview,
			ReasonCode:    "DEVICE_INTEGRITY_UNAVAILABLE",
			PolicyVersion: policyVersion,
			AppealPath:    appealPath,
		}, nil
	default:
		return IntegrityRiskDecision{}, ErrInvalidIntegrityEvidence
	}
}
