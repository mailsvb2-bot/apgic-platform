package authz

import "time"

type Decision string

const (
	Allow          Decision = "ALLOW"
	Deny           Decision = "DENY"
	RequireStepUp  Decision = "REQUIRE_STEP_UP"
)

type Principal struct {
	IdentityID string
	TenantID   string
	Scopes     map[string]struct{}
	StepUpAt   time.Time
}

type Resource struct {
	TenantID      string
	RequiredScope string
	HighRisk      bool
}

type Result struct {
	Decision Decision
	Reason   string
}

func Authorize(principal Principal, resource Resource, now time.Time, maxStepUpAge time.Duration) Result {
	if principal.IdentityID == "" {
		return Result{Decision: Deny, Reason: "AUTHENTICATION_REQUIRED"}
	}
	if principal.TenantID == "" || resource.TenantID == "" || principal.TenantID != resource.TenantID {
		return Result{Decision: Deny, Reason: "TENANT_ISOLATION_DENY"}
	}
	if resource.RequiredScope != "" {
		if _, ok := principal.Scopes[resource.RequiredScope]; !ok {
			return Result{Decision: Deny, Reason: "SCOPE_DENY"}
		}
	}
	if resource.HighRisk {
		if principal.StepUpAt.IsZero() || now.Sub(principal.StepUpAt) > maxStepUpAge {
			return Result{Decision: RequireStepUp, Reason: "STEP_UP_REQUIRED"}
		}
	}
	return Result{Decision: Allow, Reason: "AUTHORIZED"}
}
