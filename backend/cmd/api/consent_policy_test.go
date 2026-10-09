package main

import (
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/privacy"
)

func TestConsentPoliciesFromEnvironmentStagingDisabledFailClosed(t *testing.T) {
	t.Setenv("APGIC_GROWTH_CONSENT_POLICY_VERSION", "DISABLED")
	t.Setenv("APGIC_GROWTH_CONSENT_TEXT_HASH_OR_VERSION", "DISABLED")
	policies, err := consentPoliciesFromEnvironment("STAGING")
	if err != nil || policies != nil {
		t.Fatalf("disabled staging must have no consent policy: policies=%#v err=%v", policies, err)
	}
	for _, environment := range []string{"PRODUCTION", "TEST"} {
		if _, err := consentPoliciesFromEnvironment(environment); err == nil {
			t.Fatalf("%s must reject staging-only disabled consent", environment)
		}
	}
	for _, values := range [][2]string{{"DISABLED", ""}, {"", "DISABLED"}, {"DISABLED", "growth-v1"}} {
		t.Setenv("APGIC_GROWTH_CONSENT_POLICY_VERSION", values[0])
		t.Setenv("APGIC_GROWTH_CONSENT_TEXT_HASH_OR_VERSION", values[1])
		if _, err := consentPoliciesFromEnvironment("STAGING"); err == nil {
			t.Fatalf("partial staging disable must be rejected: %#v", values)
		}
	}
}

func TestConsentPoliciesFromEnvironmentFailClosedForDurableRuntime(t *testing.T) {
	t.Setenv("APGIC_GROWTH_CONSENT_POLICY_VERSION", "")
	t.Setenv("APGIC_GROWTH_CONSENT_TEXT_HASH_OR_VERSION", "")

	if _, err := consentPoliciesFromEnvironment("STAGING"); err == nil {
		t.Fatal("staging unexpectedly accepted missing growth consent policy")
	}
	if policies, err := consentPoliciesFromEnvironment("TEST"); err != nil || policies != nil {
		t.Fatalf("non-durable conformance should allow no consent policy: policies=%#v err=%v", policies, err)
	}
}

func TestConsentPoliciesFromEnvironmentRejectsIncompleteOrPlaceholderPolicy(t *testing.T) {
	cases := []struct {
		version string
		text    string
	}{
		{version: "growth-v1", text: ""},
		{version: "", text: "sha256:text"},
		{version: "CONFIG_REQUIRED", text: "sha256:text"},
		{version: "growth-v1", text: "CONFIG_REQUIRED"},
	}
	for _, tc := range cases {
		t.Run(tc.version+"/"+tc.text, func(t *testing.T) {
			t.Setenv("APGIC_GROWTH_CONSENT_POLICY_VERSION", tc.version)
			t.Setenv("APGIC_GROWTH_CONSENT_TEXT_HASH_OR_VERSION", tc.text)
			if _, err := consentPoliciesFromEnvironment("STAGING"); err == nil {
				t.Fatal("invalid growth consent policy unexpectedly accepted")
			}
		})
	}
}

func TestConsentPoliciesFromEnvironmentBuildsCanonicalGrowthPolicy(t *testing.T) {
	t.Setenv("APGIC_GROWTH_CONSENT_POLICY_VERSION", "growth-policy-v7")
	t.Setenv("APGIC_GROWTH_CONSENT_TEXT_HASH_OR_VERSION", "sha256:approved-copy")

	policies, err := consentPoliciesFromEnvironment("STAGING")
	if err != nil {
		t.Fatal(err)
	}
	policy, ok := policies[privacy.PurposeGrowthSessionProjection]
	if !ok {
		t.Fatalf("growth policy missing: %#v", policies)
	}
	if policy.PolicyVersion != "growth-policy-v7" ||
		policy.TextHashOrVersion != "sha256:approved-copy" {
		t.Fatalf("unexpected growth policy: %#v", policy)
	}
}
