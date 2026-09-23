package launchconfig

import (
	"fmt"
	"strings"
)

const ReasonConfigRequired = "CONFIG_REQUIRED"

type Config struct {
	JurisdictionMatrixVersion string
	RetentionPolicyVersion    string
	SLOPolicyVersion          string
	ProviderMatrixVersion     string
}

type Problem struct {
	Field  string
	Reason string
}

func (p Problem) Error() string {
	return fmt.Sprintf("%s: %s", p.Field, p.Reason)
}

func Preflight(config Config) []Problem {
	required := []struct {
		field string
		value string
	}{
		{"jurisdiction_matrix_version", config.JurisdictionMatrixVersion},
		{"retention_policy_version", config.RetentionPolicyVersion},
		{"slo_policy_version", config.SLOPolicyVersion},
		{"provider_matrix_version", config.ProviderMatrixVersion},
	}

	problems := make([]Problem, 0)
	for _, item := range required {
		if strings.TrimSpace(item.value) == "" || item.value == ReasonConfigRequired {
			problems = append(problems, Problem{Field: item.field, Reason: ReasonConfigRequired})
		}
	}
	return problems
}

func Ready(config Config) bool {
	return len(Preflight(config)) == 0
}
