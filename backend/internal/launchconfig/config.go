package launchconfig

import (
	"fmt"
	"strings"
)

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
	required := map[string]string{
		"jurisdiction_matrix_version": config.JurisdictionMatrixVersion,
		"retention_policy_version":    config.RetentionPolicyVersion,
		"slo_policy_version":          config.SLOPolicyVersion,
		"provider_matrix_version":     config.ProviderMatrixVersion,
	}
	problems := make([]Problem, 0)
	for field, value := range required {
		if strings.TrimSpace(value) == "" {
			problems = append(problems, Problem{Field: field, Reason: "CONFIG_REQUIRED"})
		}
	}
	return problems
}
