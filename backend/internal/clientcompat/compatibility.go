package clientcompat

import (
	"errors"
	"fmt"
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

var (
	ErrInvalidVersion = errors.New("invalid semantic version")
	ErrPolicyMissing  = errors.New("client compatibility policy missing")
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
	Platform          Platform
	MinimumSupported  Version
	Recommended       Version
	ContractVersion   string
	PolicyVersion     string
}

type Decision struct {
	Status          Status
	ReasonCode      string
	PolicyVersion   string
	ContractVersion string
}

func Evaluate(appVersion Version, policy Policy) (Decision, error) {
	if policy.Platform != IOS && policy.Platform != Android {
		return Decision{}, ErrPolicyMissing
	}
	if strings.TrimSpace(policy.PolicyVersion) == "" || strings.TrimSpace(policy.ContractVersion) == "" {
		return Decision{}, ErrPolicyMissing
	}
	if policy.Recommended.Compare(policy.MinimumSupported) < 0 {
		return Decision{}, fmt.Errorf("%w: recommended below minimum", ErrPolicyMissing)
	}

	decision := Decision{
		PolicyVersion:   policy.PolicyVersion,
		ContractVersion: policy.ContractVersion,
	}

	switch {
	case appVersion.Compare(policy.MinimumSupported) < 0:
		decision.Status = UpdateRequired
		decision.ReasonCode = "CLIENT_VERSION_BELOW_MINIMUM"
	case appVersion.Compare(policy.Recommended) < 0:
		decision.Status = DeprecatedButSupported
		decision.ReasonCode = "CLIENT_VERSION_DEPRECATED"
	default:
		decision.Status = Supported
		decision.ReasonCode = "CLIENT_VERSION_SUPPORTED"
	}
	return decision, nil
}
