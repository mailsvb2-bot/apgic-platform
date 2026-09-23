package launchconfig

import "testing"

func TestPreflightFailsClosedForMissingCriticalConfig(t *testing.T) {
	problems := Preflight(Config{
		JurisdictionMatrixVersion: "jurisdiction-v1",
		RetentionPolicyVersion:    "retention-v1",
		SLOPolicyVersion:          "slo-v1",
	})
	if len(problems) != 1 || problems[0].Field != "provider_matrix_version" {
		t.Fatalf("unexpected problems: %+v", problems)
	}
}

func TestPreflightAcceptsExplicitVersionedConfig(t *testing.T) {
	problems := Preflight(Config{
		JurisdictionMatrixVersion: "jurisdiction-v1",
		RetentionPolicyVersion:    "retention-v1",
		SLOPolicyVersion:          "slo-v1",
		ProviderMatrixVersion:     "providers-v1",
	})
	if len(problems) != 0 {
		t.Fatalf("unexpected problems: %+v", problems)
	}
}
