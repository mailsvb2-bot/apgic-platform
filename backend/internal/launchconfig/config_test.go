package launchconfig

import "testing"

func TestPreflightFailsClosedAndIsDeterministic(t *testing.T) {
	problems := Preflight(Config{})
	want := []string{
		"jurisdiction_matrix_version",
		"retention_policy_version",
		"slo_policy_version",
		"provider_matrix_version",
	}
	if len(problems) != len(want) {
		t.Fatalf("unexpected problems: %+v", problems)
	}
	for i, field := range want {
		if problems[i].Field != field || problems[i].Reason != ReasonConfigRequired {
			t.Fatalf("problem %d=%+v", i, problems[i])
		}
	}
}

func TestPlaceholderIsNotReady(t *testing.T) {
	config := Config{
		JurisdictionMatrixVersion: "j-v1",
		RetentionPolicyVersion:    "r-v1",
		SLOPolicyVersion:          "s-v1",
		ProviderMatrixVersion:     ReasonConfigRequired,
	}
	if Ready(config) {
		t.Fatal("CONFIG_REQUIRED must fail closed")
	}
}

func TestExplicitVersionedConfigIsReady(t *testing.T) {
	config := Config{
		JurisdictionMatrixVersion: "j-v1",
		RetentionPolicyVersion:    "r-v1",
		SLOPolicyVersion:          "s-v1",
		ProviderMatrixVersion:     "p-v1",
	}
	if !Ready(config) {
		t.Fatal("versioned config should be ready")
	}
}
