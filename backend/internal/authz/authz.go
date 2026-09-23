package authz

import "time"

type Risk string

const (
	RiskNormal Risk = "NORMAL"
	RiskHigh   Risk = "HIGH_RISK"
)

type Decision string

const (
	Allow          Decision = "ALLOW"
	Deny           Decision = "DENY"
	StepUpRequired Decision = "STEP_UP_REQUIRED"
)

type Principal struct {
	ID          string
	TenantID    string
	Permissions map[string]struct{}
	StepUpAt    *time.Time
}

type ResourceRef struct {
	ID       string
	TenantID string
}

type Input struct {
	Principal    Principal
	Resource     ResourceRef
	Action       string
	Risk         Risk
	Now          time.Time
	MaxStepUpAge time.Duration
}

type Result struct {
	Decision   Decision
	ReasonCode string
}

func Authorize(in Input) Result {
	if in.Principal.ID == "" {
		return Result{Decision: Deny, ReasonCode: "AUTH_PRINCIPAL_REQUIRED"}
	}
	if in.Now.IsZero() {
		return Result{Decision: Deny, ReasonCode: "AUTH_TIME_REQUIRED"}
	}
	if in.Risk != RiskNormal && in.Risk != RiskHigh {
		return Result{Decision: Deny, ReasonCode: "AUTH_RISK_INVALID"}
	}
	if in.Resource.TenantID == "" || in.Principal.TenantID == "" || in.Resource.TenantID != in.Principal.TenantID {
		return Result{Decision: Deny, ReasonCode: "AUTH_CROSS_TENANT_DENY"}
	}
	if _, ok := in.Principal.Permissions[in.Action]; !ok {
		return Result{Decision: Deny, ReasonCode: "AUTH_PERMISSION_DENIED"}
	}
	if in.Risk == RiskHigh {
		maxAge := in.MaxStepUpAge
		if maxAge <= 0 {
			maxAge = 10 * time.Minute
		}
		if in.Principal.StepUpAt == nil {
			return Result{Decision: StepUpRequired, ReasonCode: "AUTH_STEP_UP_REQUIRED"}
		}
		if in.Principal.StepUpAt.After(in.Now) {
			return Result{Decision: StepUpRequired, ReasonCode: "AUTH_STEP_UP_INVALID_TIME"}
		}
		if in.Now.Sub(*in.Principal.StepUpAt) > maxAge {
			return Result{Decision: StepUpRequired, ReasonCode: "AUTH_STEP_UP_REQUIRED"}
		}
	}
	return Result{Decision: Allow, ReasonCode: "AUTH_ALLOWED"}
}
