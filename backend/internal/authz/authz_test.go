package authz

import (
	"testing"
	"time"
)

func normalInput(now time.Time) Input {
	return Input{
		Principal: Principal{
			ID: "alice", TenantID: "org-a",
			Permissions: map[string]struct{}{"organization.read_private": {}},
		},
		Resource: ResourceRef{ID: "org-a", TenantID: "org-a"},
		Action:   "organization.read_private",
		Risk:     RiskNormal,
		Now:      now,
	}
}

func TestCrossTenantKnownResourceIDIsDenied(t *testing.T) {
	in := normalInput(time.Now())
	in.Resource = ResourceRef{ID: "known-uuid", TenantID: "org-b"}
	got := Authorize(in)
	if got.Decision != Deny || got.ReasonCode != "AUTH_CROSS_TENANT_DENY" {
		t.Fatalf("unexpected decision: %#v", got)
	}
}

func TestUnknownRiskFailsClosed(t *testing.T) {
	in := normalInput(time.Now())
	in.Risk = Risk("UNRECOGNIZED")
	got := Authorize(in)
	if got.Decision != Deny || got.ReasonCode != "AUTH_RISK_INVALID" {
		t.Fatalf("unexpected decision: %#v", got)
	}
}

func TestHighRiskRequiresFreshNonFutureStepUp(t *testing.T) {
	now := time.Now()
	in := normalInput(now)
	in.Principal.ID = "owner"
	in.Principal.Permissions = map[string]struct{}{"organization.transfer_ownership": {}}
	in.Action = "organization.transfer_ownership"
	in.Risk = RiskHigh

	got := Authorize(in)
	if got.Decision != StepUpRequired {
		t.Fatalf("expected step-up, got %#v", got)
	}

	future := now.Add(time.Minute)
	in.Principal.StepUpAt = &future
	got = Authorize(in)
	if got.Decision != StepUpRequired || got.ReasonCode != "AUTH_STEP_UP_INVALID_TIME" {
		t.Fatalf("future step-up must fail closed, got %#v", got)
	}

	fresh := now.Add(-time.Minute)
	in.Principal.StepUpAt = &fresh
	got = Authorize(in)
	if got.Decision != Allow {
		t.Fatalf("expected allow after fresh step-up, got %#v", got)
	}
}
