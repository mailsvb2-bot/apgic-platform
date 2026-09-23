package authz

import (
	"testing"
	"time"
)

func TestCrossTenantKnownResourceIDIsDenied(t *testing.T) {
	got := Authorize(Input{
		Principal: Principal{
			ID: "alice", TenantID: "org-a",
			Permissions: map[string]struct{}{"organization.read_private": {}},
		},
		Resource: ResourceRef{ID: "known-uuid", TenantID: "org-b"},
		Action:   "organization.read_private",
		Now:      time.Now(),
	})
	if got.Decision != Deny || got.ReasonCode != "AUTH_CROSS_TENANT_DENY" {
		t.Fatalf("unexpected decision: %#v", got)
	}
}

func TestHighRiskRequiresFreshStepUp(t *testing.T) {
	now := time.Now()
	got := Authorize(Input{
		Principal: Principal{
			ID: "owner", TenantID: "org-a",
			Permissions: map[string]struct{}{"organization.transfer_ownership": {}},
		},
		Resource: ResourceRef{ID: "org-a", TenantID: "org-a"},
		Action:   "organization.transfer_ownership",
		Risk:     RiskHigh,
		Now:      now,
	})
	if got.Decision != StepUpRequired {
		t.Fatalf("expected step-up, got %#v", got)
	}

	step := now.Add(-time.Minute)
	got = Authorize(Input{
		Principal: Principal{
			ID: "owner", TenantID: "org-a", StepUpAt: &step,
			Permissions: map[string]struct{}{"organization.transfer_ownership": {}},
		},
		Resource: ResourceRef{ID: "org-a", TenantID: "org-a"},
		Action:   "organization.transfer_ownership",
		Risk:     RiskHigh,
		Now:      now,
	})
	if got.Decision != Allow {
		t.Fatalf("expected allow after fresh step-up, got %#v", got)
	}
}
