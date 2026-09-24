package security

import (
	"errors"
	"testing"
	"time"
)

func TestNegativeIntegritySignalDoesNotBecomeAutomaticBan(t *testing.T) {
	evidence := DeviceIntegrityEvidence{
		ID: "integrity-1",
		IdentityID: "identity-1",
		InstallationID: "installation-1",
		ProviderKind: "PROVIDER_A",
		ProviderEvidenceRef: "evidence/integrity-1",
		Verdict: IntegrityNegative,
		ServerVerified: true,
		ObservedAt: time.Now().UTC(),
	}
	decision, err := EvaluateIntegrity(evidence, "integrity-policy-v1", "/support/integrity-appeal")
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != IntegrityStepUp {
		t.Fatalf("negative integrity must require step-up/review rather than automatic ban: %#v", decision)
	}
}

func TestUnsupportedIntegrityHasGracefulTypedPath(t *testing.T) {
	evidence := DeviceIntegrityEvidence{
		ID: "integrity-2",
		IdentityID: "identity-1",
		InstallationID: "installation-2",
		Verdict: IntegrityUnsupported,
		ObservedAt: time.Now().UTC(),
	}
	decision, err := EvaluateIntegrity(evidence, "integrity-policy-v1", "/support/integrity-appeal")
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != IntegrityReview || decision.ReasonCode != "DEVICE_INTEGRITY_UNSUPPORTED" {
		t.Fatalf("unsupported integrity must degrade gracefully: %#v", decision)
	}
}

func TestVerifiedIntegrityEvidenceRequiresProviderEvidence(t *testing.T) {
	evidence := DeviceIntegrityEvidence{
		ID: "integrity-3",
		IdentityID: "identity-1",
		InstallationID: "installation-3",
		ProviderKind: "PROVIDER_A",
		Verdict: IntegrityValid,
		ServerVerified: true,
		ObservedAt: time.Now().UTC(),
	}
	if !errors.Is(evidence.Validate(), ErrInvalidIntegrityEvidence) {
		t.Fatal("verified signal without provider evidence must be rejected")
	}
}
