package launchconfig

import (
	"fmt"
	"strings"
)

const ReasonConfigRequired = "CONFIG_REQUIRED"

type Config struct {
	JurisdictionMatrixVersion string
	RetentionPolicyVersion string
	SLOPolicyVersion string
	ProviderMatrixVersion string
}
type Problem struct { Field string; Reason string }
func (p Problem) Error() string { return fmt.Sprintf("%s: %s",p.Field,p.Reason) }

func Preflight(c Config) []Problem {
	required:=[]struct{field,value string}{
		{"jurisdiction_matrix_version",c.JurisdictionMatrixVersion},
		{"retention_policy_version",c.RetentionPolicyVersion},
		{"slo_policy_version",c.SLOPolicyVersion},
		{"provider_matrix_version",c.ProviderMatrixVersion},
	}
	out:=make([]Problem,0)
	for _,item:=range required {
		if strings.TrimSpace(item.value)=="" || item.value==ReasonConfigRequired {
			out=append(out,Problem{Field:item.field,Reason:ReasonConfigRequired})
		}
	}
	return out
}
func Ready(c Config) bool { return len(Preflight(c))==0 }
