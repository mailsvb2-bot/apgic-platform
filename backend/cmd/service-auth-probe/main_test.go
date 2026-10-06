package main

import (
	"testing"
	"time"
)

func TestRunProbeProvesServiceCredentialLifecycleAndScope(t *testing.T) {
	out, err := runProbe(time.Date(2026, 10, 5, 19, 0, 0, 0, time.UTC), "candidate")
	if err != nil {
		t.Fatal(err)
	}
	if !out.ActiveCredentialAccepted || !out.WrongSecretDenied || !out.RevokedCredentialDenied ||
		!out.ExpiredCredentialDenied || !out.ExactScopeExecutionPassed || !out.WrongScopeDenied {
		t.Fatalf("incomplete probe evidence: %#v", out)
	}
	if out.ProviderCalls != 1 {
		t.Fatalf("provider calls=%d want=1", out.ProviderCalls)
	}
	if out.SecretMaterialEmitted {
		t.Fatal("probe must never emit credential material")
	}
}
