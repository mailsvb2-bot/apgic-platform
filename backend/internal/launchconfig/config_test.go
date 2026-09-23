package launchconfig

import "testing"

func TestPreflightFailsClosedAndIsDeterministic(t *testing.T) {
	p:=Preflight(Config{})
	want:=[]string{"jurisdiction_matrix_version","retention_policy_version","slo_policy_version","provider_matrix_version"}
	if len(p)!=len(want) { t.Fatalf("unexpected problems: %+v",p) }
	for i,field:=range want { if p[i].Field!=field || p[i].Reason!=ReasonConfigRequired { t.Fatalf("problem %d=%+v",i,p[i]) } }
}
func TestPlaceholderIsNotReady(t *testing.T) {
	if Ready(Config{JurisdictionMatrixVersion:"j-v1",RetentionPolicyVersion:"r-v1",SLOPolicyVersion:"s-v1",ProviderMatrixVersion:ReasonConfigRequired}) { t.Fatal("CONFIG_REQUIRED must fail closed") }
}
func TestExplicitVersionedConfigIsReady(t *testing.T) {
	if !Ready(Config{JurisdictionMatrixVersion:"j-v1",RetentionPolicyVersion:"r-v1",SLOPolicyVersion:"s-v1",ProviderMatrixVersion:"p-v1"}) { t.Fatal("versioned config should be ready") }
}
