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

func TestCompatibilityWindowIsDeterministic(t *testing.T) {
	policy := Policy{
		Platform:         IOS,
		MinimumSupported: mustVersion(t, "1.4.0"),
		Recommended:      mustVersion(t, "1.6.0"),
		ContractVersion:  "contract-v1",
		PolicyVersion:    "mobile-compat-v7",
	}

	cases := []struct {
		version string
		status  Status
		reason  string
	}{
		{"1.3.9", UpdateRequired, "CLIENT_VERSION_BELOW_MINIMUM"},
		{"1.4.0", DeprecatedButSupported, "CLIENT_VERSION_DEPRECATED"},
		{"1.5.9", DeprecatedButSupported, "CLIENT_VERSION_DEPRECATED"},
		{"1.6.0", Supported, "CLIENT_VERSION_SUPPORTED"},
		{"2.0.0", Supported, "CLIENT_VERSION_SUPPORTED"},
	}

	for _, tc := range cases {
		decision, err := Evaluate(mustVersion(t, tc.version), policy)
		if err != nil {
			t.Fatal(err)
		}
		if decision.Status != tc.status || decision.ReasonCode != tc.reason {
			t.Fatalf("%s: got %+v", tc.version, decision)
		}
		if decision.PolicyVersion != "mobile-compat-v7" || decision.ContractVersion != "contract-v1" {
			t.Fatalf("%s: policy evidence missing: %+v", tc.version, decision)
		}
	}
}

func TestInvalidOrMissingPolicyFailsClosed(t *testing.T) {
	_, err := Evaluate(mustVersion(t, "1.0.0"), Policy{})
	if err == nil {
		t.Fatal("missing compatibility policy must fail closed")
	}
}

func TestParseVersionRejectsAmbiguousInput(t *testing.T) {
	for _, raw := range []string{"1", "1.2", "1.2.x", "-1.2.3", ""} {
		if _, err := ParseVersion(raw); err == nil {
			t.Fatalf("expected %q to fail", raw)
		}
	}
}
