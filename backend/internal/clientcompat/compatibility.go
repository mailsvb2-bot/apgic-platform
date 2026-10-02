package clientcompat

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type Platform string

const (
	IOS     Platform = "IOS"
	Android Platform = "ANDROID"
)

type Status string

const (
	Supported              Status = "SUPPORTED"
	DeprecatedButSupported Status = "DEPRECATED_BUT_SUPPORTED"
	UpdateRequired         Status = "UPDATE_REQUIRED"
)

type ForcedUpdateReason string

const (
	SecurityCritical     ForcedUpdateReason = "SECURITY_CRITICAL"
	LegalCritical        ForcedUpdateReason = "LEGAL_CRITICAL"
	IncompatibleCritical ForcedUpdateReason = "INCOMPATIBLE_CRITICAL"
)

const (
	ReasonClientVersionSupported    = "CLIENT_VERSION_SUPPORTED"
	ReasonClientVersionDeprecated   = "CLIENT_VERSION_DEPRECATED"
	ReasonClientVersionBelowMin     = "CLIENT_VERSION_BELOW_MINIMUM"
	ReasonClientBuildBelowMin       = "CLIENT_BUILD_BELOW_MINIMUM"
	ReasonClientContractUnsupported = "CLIENT_CONTRACT_UNSUPPORTED"
)

var (
	ErrInvalidVersion  = errors.New("invalid semantic version")
	ErrInvalidBuild    = errors.New("invalid client build number")
	ErrInvalidContract = errors.New("invalid client contract version")
	ErrPolicyMissing   = errors.New("client compatibility policy missing")
)

type Version struct {
	Major int
	Minor int
	Patch int
}

func ParseVersion(raw string) (Version, error) {
	raw = strings.TrimSpace(strings.TrimPrefix(raw, "v"))
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return Version{}, ErrInvalidVersion
	}

	values := make([]int, 3)
	for i, part := range parts {
		if part == "" {
			return Version{}, ErrInvalidVersion
		}
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return Version{}, ErrInvalidVersion
		}
		values[i] = value
	}
	return Version{Major: values[0], Minor: values[1], Patch: values[2]}, nil
}

func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		return compareInt(v.Major, other.Major)
	}
	if v.Minor != other.Minor {
		return compareInt(v.Minor, other.Minor)
	}
	return compareInt(v.Patch, other.Patch)
}

func compareInt(left, right int) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

type Policy struct {
	Platform                  Platform
	MinimumSupported          Version
	Recommended               Version
	MinimumBuild              int
	ContractVersion           string
	SupportedContractVersions []string
	PolicyVersion             string
	MinimumUpdateReason       ForcedUpdateReason
	UpdateURL                 string
}

type Decision struct {
	Status          Status             `json:"status"`
	ReasonCode      string             `json:"reason_code"`
	PolicyVersion   string             `json:"policy_version"`
	ContractVersion string             `json:"contract_version"`
	UpdateReason    ForcedUpdateReason `json:"update_reason,omitempty"`
	UpdateURL       string             `json:"update_url,omitempty"`
}

func Evaluate(appVersion Version, policy Policy) (Decision, error) {
	return EvaluateClient(appVersion, policy.MinimumBuild, policy.ContractVersion, policy)
}

func EvaluateClient(appVersion Version, buildNumber int, clientContractVersion string, policy Policy) (Decision, error) {
	if err := validatePolicy(policy); err != nil {
		return Decision{}, err
	}
	if buildNumber <= 0 {
		return Decision{}, ErrInvalidBuild
	}
	clientContractVersion = strings.TrimSpace(clientContractVersion)
	if clientContractVersion == "" {
		return Decision{}, ErrInvalidContract
	}

	decision := Decision{
		PolicyVersion:   policy.PolicyVersion,
		ContractVersion: policy.ContractVersion,
	}

	if !supportsContract(policy, clientContractVersion) {
		decision.Status = UpdateRequired
		decision.ReasonCode = ReasonClientContractUnsupported
		decision.UpdateReason = IncompatibleCritical
		decision.UpdateURL = policy.UpdateURL
		return decision, nil
	}

	switch {
	case appVersion.Compare(policy.MinimumSupported) < 0:
		decision.Status = UpdateRequired
		decision.ReasonCode = ReasonClientVersionBelowMin
		decision.UpdateReason = policy.MinimumUpdateReason
		decision.UpdateURL = policy.UpdateURL
	case buildNumber < policy.MinimumBuild:
		decision.Status = UpdateRequired
		decision.ReasonCode = ReasonClientBuildBelowMin
		decision.UpdateReason = policy.MinimumUpdateReason
		decision.UpdateURL = policy.UpdateURL
	case appVersion.Compare(policy.Recommended) < 0:
		decision.Status = DeprecatedButSupported
		decision.ReasonCode = ReasonClientVersionDeprecated
	default:
		decision.Status = Supported
		decision.ReasonCode = ReasonClientVersionSupported
	}
	return decision, nil
}

func validatePolicy(policy Policy) error {
	if policy.Platform != IOS && policy.Platform != Android {
		return ErrPolicyMissing
	}
	if strings.TrimSpace(policy.PolicyVersion) == "" || strings.TrimSpace(policy.ContractVersion) == "" {
		return ErrPolicyMissing
	}
	if policy.Recommended.Compare(policy.MinimumSupported) < 0 {
		return fmt.Errorf("%w: recommended below minimum", ErrPolicyMissing)
	}
	if policy.MinimumBuild <= 0 {
		return fmt.Errorf("%w: minimum build must be positive", ErrPolicyMissing)
	}
	switch policy.MinimumUpdateReason {
	case SecurityCritical, LegalCritical, IncompatibleCritical:
	default:
		return fmt.Errorf("%w: invalid forced-update reason", ErrPolicyMissing)
	}
	parsed, err := url.Parse(strings.TrimSpace(policy.UpdateURL))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return fmt.Errorf("%w: update URL must be https", ErrPolicyMissing)
	}
	if !supportsContract(policy, policy.ContractVersion) {
		return fmt.Errorf("%w: current contract must remain supported", ErrPolicyMissing)
	}
	return nil
}

func supportsContract(policy Policy, contractVersion string) bool {
	contractVersion = strings.TrimSpace(contractVersion)
	if contractVersion == "" {
		return false
	}
	supported := policy.SupportedContractVersions
	if len(supported) == 0 {
		supported = []string{policy.ContractVersion}
	}
	for _, candidate := range supported {
		if strings.TrimSpace(candidate) == contractVersion {
			return true
		}
	}
	return false
}
