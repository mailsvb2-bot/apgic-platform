package main

import (
	"strings"
	"testing"
)

func TestGrowthRequiresExplicitEnabledAndIndependentStrongKey(t *testing.T) {
	cases := []struct {
		name, enabled, key string
		accept             bool
	}{
		{name: "disabled", enabled: "0", accept: true},
		{name: "legacy disabled", accept: true},
		{name: "on without secret", enabled: "1"},
		{name: "on short secret", enabled: "1", key: "short"},
		{name: "secret silently disabled", key: strings.Repeat("a", 40)},
		{name: "invalid toggle", enabled: "TRUE"},
		{name: "on strong key", enabled: "1", key: strings.Repeat("b", 40), accept: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("APGIC_GROWTH_ENABLED", tc.enabled)
			t.Setenv("APGIC_GROWTH_SIGNING_KEY", tc.key)
			key, err := growthSigningKeyFromEnvironment()
			if (err == nil) != tc.accept {
				t.Fatalf("toggle=%q key_len=%d error=%v", tc.enabled, len(tc.key), err)
			}
			if tc.enabled == "1" && tc.accept && string(key) != tc.key {
				t.Fatal("enabled signing key not forwarded intact")
			}
			if tc.enabled != "1" && len(key) != 0 {
				t.Fatal("disabled Growth exposed signing key")
			}
		})
	}
}
