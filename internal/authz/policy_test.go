package authz

import (
	"testing"
	"time"
)

func TestCrossTenantUUIDKnowledgeNeverAuthorizes(t *testing.T) {
	now := time.Now()
	result := Authorize(
		Principal{IdentityID: "user-a", TenantID: "org-a", Scopes: map[string]struct{}{"resource:read": {}}},
		Resource{TenantID: "org-b", RequiredScope: "resource:read"},
		now,
		10*time.Minute,
	)
	if result.Decision != Deny || result.Reason != "TENANT_ISOLATION_DENY" {
		t.Fatalf("unexpected decision: %+v", result)
	}
}

func TestHighRiskActionRequiresFreshStepUp(t *testing.T) {
	now := time.Now()
	result := Authorize(
		Principal{IdentityID: "user-a", TenantID: "org-a", Scopes: map[string]struct{}{"payout:write": {}}, StepUpAt: now.Add(-time.Hour)},
		Resource{TenantID: "org-a", RequiredScope: "payout:write", HighRisk: true},
		now,
		10*time.Minute,
	)
	if result.Decision != RequireStepUp || result.Reason != "STEP_UP_REQUIRED" {
		t.Fatalf("unexpected decision: %+v", result)
	}
}
