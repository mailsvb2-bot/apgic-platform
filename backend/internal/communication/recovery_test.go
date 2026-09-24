package communication

import (
	"testing"
	"time"
)

func recoveryFixture() (TechnicalFailure, RecoveryPolicy, RecoveryContext) {
	failure := TechnicalFailure{
		ConsultationID: "consultation-1",
		BookingID: "booking-1",
		ProviderInstanceID: "communication-a",
		Kind: FailureProviderDisconnect,
		Attempt: 0,
		EvidenceRef: "provider-evidence/disconnect-1",
		OccurredAt: time.Date(2026, 9, 24, 12, 30, 0, 0, time.UTC),
	}
	policy := RecoveryPolicy{
		Version: "communication-recovery-v1",
		MaxRecoveryAttempts: 2,
		AllowFallback: true,
		ExhaustedAction: RecoveryRefund,
	}
	ctx := RecoveryContext{
		CurrentProviderID: "communication-a",
		FallbackProviderID: "communication-b",
		RefundEligible: true,
		RescheduleAvailable: true,
	}
	return failure, policy, ctx
}

func TestRecoveryUsesProviderNeutralFallbackBeforeFinancialRecovery(t *testing.T) {
	failure, policy, ctx := recoveryFixture()
	decision, err := DecideRecovery(failure, policy, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != RecoveryFallbackProvider ||
		decision.ProviderInstanceID != "communication-b" ||
		decision.RefundPathRequired ||
		decision.RescheduleRequired {
		t.Fatalf("unexpected recovery decision: %#v", decision)
	}
}

func TestRecoveryExhaustionStartsExplicitRefundPath(t *testing.T) {
	failure, policy, ctx := recoveryFixture()
	failure.Attempt = 2
	decision, err := DecideRecovery(failure, policy, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != RecoveryRefund || !decision.RefundPathRequired {
		t.Fatalf("unexpected exhausted recovery decision: %#v", decision)
	}
	if decision.ProviderInstanceID != "" {
		t.Fatalf("financial recovery must not silently execute through communication provider: %#v", decision)
	}
}

func TestRecoveryCanPreferRescheduleWhenPolicyRequiresIt(t *testing.T) {
	failure, policy, ctx := recoveryFixture()
	failure.Attempt = 2
	policy.ExhaustedAction = RecoveryReschedule
	decision, err := DecideRecovery(failure, policy, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != RecoveryReschedule || !decision.RescheduleRequired {
		t.Fatalf("unexpected reschedule decision: %#v", decision)
	}
}
