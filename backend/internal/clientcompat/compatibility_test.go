package clientcompat

import "testing"

func mustVersion(t *testing.T, raw string) Version {
	t.Helper()
	version, err := ParseVersion(raw)
	if err != nil {
		t.Fatalf("ParseVersion(%q): %v", raw, err)
	}
	return version
}

func testPolicy(t *testing.T, platform Platform) Policy {
	t.Helper()
	return Policy{
		Platform:                  platform,
		MinimumSupported:          mustVersion(t, "1.4.0"),
		Recommended:               mustVersion(t, "1.6.0"),
		ContractVersion:           "contract-v2",
		SupportedContractVersions: []string{"contract-v1", "contract-v2"},
		PolicyVersion:             "mobile-compat-v7",
		MinimumUpdateReason:       IncompatibleCritical,
		UpdateURL:                 "https://apgic.ru/update",
	}
}

func TestCompatibilityWindowIsDeterministic(t *testing.T) {
	policy := testPolicy(t, IOS)

	cases := []struct {
		version string
		status  Status
		reason  string
	}{
		{"1.3.9", UpdateRequired, ReasonClientVersionBelowMin},
		{"1.4.0", DeprecatedButSupported, ReasonClientVersionDeprecated},
		{"1.5.9", DeprecatedButSupported, ReasonClientVersionDeprecated},
		{"1.6.0", Supported, ReasonClientVersionSupported},
		{"2.0.0", Supported, ReasonClientVersionSupported},
	}

	for _, tc := range cases {
		decision, err := EvaluateClient(mustVersion(t, tc.version), "contract-v1", policy)
		if err != nil {
			t.Fatal(err)
		}
		if decision.Status != tc.status || decision.ReasonCode != tc.reason {
			t.Fatalf("%s: got %+v", tc.version, decision)
		}
		if decision.PolicyVersion != "mobile-compat-v7" || decision.ContractVersion != "contract-v2" {
			t.Fatalf("%s: policy evidence missing: %+v", tc.version, decision)
		}
		if tc.status == UpdateRequired {
			if decision.UpdateReason != IncompatibleCritical || decision.UpdateURL != "https://apgic.ru/update" {
				t.Fatalf("%s: governed forced-update evidence missing: %+v", tc.version, decision)
			}
		} else if decision.UpdateReason != "" || decision.UpdateURL != "" {
			t.Fatalf("%s: non-blocking decision leaked forced-update fields: %+v", tc.version, decision)
		}
	}
}

func TestSupportedPreviousContractSurvivesBackendUpdate(t *testing.T) {
	decision, err := EvaluateClient(mustVersion(t, "1.5.0"), "contract-v1", testPolicy(t, Android))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != DeprecatedButSupported || decision.ReasonCode != ReasonClientVersionDeprecated {
		t.Fatalf("previous supported contract must keep working: %+v", decision)
	}
}

func TestUnsupportedContractGetsGovernedCriticalUpdate(t *testing.T) {
	decision, err := EvaluateClient(mustVersion(t, "1.6.0"), "contract-v0", testPolicy(t, IOS))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != UpdateRequired ||
		decision.ReasonCode != ReasonClientContractUnsupported ||
		decision.UpdateReason != IncompatibleCritical ||
		decision.UpdateURL != "https://apgic.ru/update" {
		t.Fatalf("unsupported contract must use governed update path: %+v", decision)
	}
}

func TestInvalidOrMissingPolicyFailsClosed(t *testing.T) {
	_, err := Evaluate(mustVersion(t, "1.0.0"), Policy{})
	if err == nil {
		t.Fatal("missing compatibility policy must fail closed")
	}

	policy := testPolicy(t, IOS)
	policy.MinimumUpdateReason = "MARKETING"
	if _, err := EvaluateClient(mustVersion(t, "1.0.0"), "contract-v1", policy); err == nil {
		t.Fatal("arbitrary forced-update reason must fail closed")
	}

	policy = testPolicy(t, IOS)
	policy.UpdateURL = "http://example.test/update"
	if _, err := EvaluateClient(mustVersion(t, "1.0.0"), "contract-v1", policy); err == nil {
		t.Fatal("non-https update URL must fail closed")
	}
}

func TestParseVersionRejectsAmbiguousInput(t *testing.T) {
	for _, raw := range []string{"1", "1.2", "1.2.x", "-1.2.3", ""} {
		if _, err := ParseVersion(raw); err == nil {
			t.Fatalf("expected %q to fail", raw)
		}
	}
}
