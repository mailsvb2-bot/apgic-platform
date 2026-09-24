package payments

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/audit"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/authz"
)

type AdminAction string

const (
	AdminConnect          AdminAction = "CONNECT"
	AdminTest             AdminAction = "TEST"
	AdminActivate         AdminAction = "ACTIVATE"
	AdminPrioritize       AdminAction = "PRIORITIZE"
	AdminScope            AdminAction = "SCOPE"
	AdminPause            AdminAction = "PAUSE"
	AdminRotateCredential AdminAction = "ROTATE_CREDENTIAL"
	AdminDisable          AdminAction = "DISABLE"
)

type GuardrailAction string

const (
	GuardrailAllowNewAttempts GuardrailAction = "ALLOW_NEW_ATTEMPTS"
	GuardrailBlockNewAttempts GuardrailAction = "BLOCK_NEW_ATTEMPTS"
)

var (
	ErrInvalidAdminChange      = errors.New("invalid payment provider admin change")
	ErrAdminChangeNotAuthorized = errors.New("payment provider admin change is not authorized")
	ErrAdminAuditUnavailable    = errors.New("payment provider admin change audit unavailable")
	ErrInvalidHealthSnapshot    = errors.New("invalid payment provider health snapshot")
)

type ProviderConfigSnapshot struct {
	ProviderID           ProviderID     `json:"provider_id"`
	ConfigVersion        string         `json:"config_version"`
	Status               ProviderStatus `json:"status"`
	Priority             int            `json:"priority"`
	RoutingWeightBPS     int            `json:"routing_weight_bps"`
	Jurisdictions        []string       `json:"jurisdictions"`
	Currencies           []string       `json:"currencies"`
	Methods              []MethodCode   `json:"methods"`
	Rails                []RailCode     `json:"rails"`
	CredentialVersionRef string         `json:"credential_version_ref"`
	EffectiveFrom        time.Time      `json:"effective_from"`
}

func (s ProviderConfigSnapshot) Validate() error {
	if strings.TrimSpace(string(s.ProviderID)) == "" ||
		strings.TrimSpace(s.ConfigVersion) == "" ||
		s.Priority < 0 ||
		s.RoutingWeightBPS < 0 ||
		s.RoutingWeightBPS > 10000 ||
		len(s.Jurisdictions) == 0 ||
		len(s.Currencies) == 0 ||
		len(s.Methods) == 0 ||
		len(s.Rails) == 0 ||
		strings.TrimSpace(s.CredentialVersionRef) == "" ||
		s.EffectiveFrom.IsZero() {
		return ErrInvalidAdminChange
	}
	switch s.Status {
	case ProviderActive, ProviderDegraded, ProviderPaused, ProviderDisabled:
	default:
		return ErrInvalidAdminChange
	}
	for _, currency := range s.Currencies {
		if len(currency) != 3 || currency != strings.ToUpper(currency) {
			return ErrInvalidAdminChange
		}
	}
	return nil
}

type AdminChangeRequest struct {
	Action              AdminAction
	ActorID             string
	Reason              string
	ConfigAuditRecordID string
	NewConfig           ProviderConfigSnapshot
	Now                 time.Time
}

type ControlPlane struct {
	PolicyVersion string
	Authorizer    authz.Evaluator
	Appender      audit.Appender
}

func (c ControlPlane) PlanChange(
	authInput authz.Input,
	current ProviderConfigSnapshot,
	request AdminChangeRequest,
) (ProviderConfigSnapshot, error) {
	if current.Validate() != nil ||
		request.NewConfig.Validate() != nil ||
		strings.TrimSpace(request.ActorID) == "" ||
		strings.TrimSpace(request.Reason) == "" ||
		strings.TrimSpace(request.ConfigAuditRecordID) == "" ||
		request.Now.IsZero() ||
		request.NewConfig.ProviderID != current.ProviderID ||
		request.NewConfig.ConfigVersion == current.ConfigVersion ||
		request.NewConfig.EffectiveFrom.Before(request.Now) ||
		request.NewConfig.EffectiveFrom.Before(current.EffectiveFrom) ||
		!actionMatchesChange(request.Action, current, request.NewConfig) {
		return ProviderConfigSnapshot{}, ErrInvalidAdminChange
	}
	if strings.TrimSpace(c.PolicyVersion) == "" || c.Appender == nil {
		return ProviderConfigSnapshot{}, ErrAdminAuditUnavailable
	}

	authInput.Action = "payment.provider.manage"
	authInput.Risk = authz.RiskHigh
	authInput.Now = request.Now
	result, err := c.Authorizer.Authorize(authInput)
	if err != nil {
		return ProviderConfigSnapshot{}, fmt.Errorf("%w: %v", ErrAdminAuditUnavailable, err)
	}
	if result.Decision != authz.Allow {
		return ProviderConfigSnapshot{}, ErrAdminChangeNotAuthorized
	}

	oldState, err := json.Marshal(current)
	if err != nil {
		return ProviderConfigSnapshot{}, fmt.Errorf("%w: %v", ErrAdminAuditUnavailable, err)
	}
	newState, err := json.Marshal(request.NewConfig)
	if err != nil {
		return ProviderConfigSnapshot{}, fmt.Errorf("%w: %v", ErrAdminAuditUnavailable, err)
	}
	record, err := audit.New(audit.Record{
		ID:            request.ConfigAuditRecordID,
		ActorID:       request.ActorID,
		Action:        "payment.provider.config_change",
		Scope:         authInput.Principal.TenantID,
		ResourceRef:   "payment-provider/" + string(current.ProviderID),
		OldState:      oldState,
		NewState:      newState,
		Reason:        request.Reason,
		PolicyVersion: c.PolicyVersion,
		OccurredAt:    request.Now,
		CorrelationID: authInput.CorrelationID,
	})
	if err != nil {
		return ProviderConfigSnapshot{}, fmt.Errorf("%w: %v", ErrAdminAuditUnavailable, err)
	}
	if err := c.Appender.Append(record); err != nil {
		return ProviderConfigSnapshot{}, fmt.Errorf("%w: %v", ErrAdminAuditUnavailable, err)
	}

	return request.NewConfig, nil
}

func actionMatchesChange(action AdminAction, current, next ProviderConfigSnapshot) bool {
	switch action {
	case AdminTest:
		return current == next
	case AdminActivate:
		return next.Status == ProviderActive
	case AdminPrioritize:
		return next.Priority != current.Priority || next.RoutingWeightBPS != current.RoutingWeightBPS
	case AdminScope:
		return !stringSlicesEqual(current.Jurisdictions, next.Jurisdictions) ||
			!stringSlicesEqual(current.Currencies, next.Currencies) ||
			!methodsEqual(current.Methods, next.Methods) ||
			!railsEqual(current.Rails, next.Rails)
	case AdminPause:
		return next.Status == ProviderPaused
	case AdminRotateCredential:
		return next.CredentialVersionRef != current.CredentialVersionRef
	case AdminDisable:
		return next.Status == ProviderDisabled
	case AdminConnect:
		return false
	default:
		return false
	}
}

type ProviderHealthSnapshot struct {
	ProviderID                    ProviderID      `json:"provider_id"`
	ConfigVersion                 string          `json:"config_version"`
	Health                        ProviderHealth  `json:"health"`
	ConversionRateBPS             int             `json:"conversion_rate_bps"`
	LatencyP95MS                  int             `json:"latency_p95_ms"`
	ProviderReportedFeeBPS        int             `json:"provider_reported_fee_bps"`
	ReconciliationPendingCount    int             `json:"reconciliation_pending_count"`
	ReconciliationMismatchCount   int             `json:"reconciliation_mismatch_count"`
	Guardrail                     GuardrailAction `json:"guardrail"`
	EvidenceRefs                  []string        `json:"evidence_refs"`
	ObservedAt                    time.Time       `json:"observed_at"`
}

func (s ProviderHealthSnapshot) Validate() error {
	if strings.TrimSpace(string(s.ProviderID)) == "" ||
		strings.TrimSpace(s.ConfigVersion) == "" ||
		s.ConversionRateBPS < 0 ||
		s.ConversionRateBPS > 10000 ||
		s.LatencyP95MS < 0 ||
		s.ProviderReportedFeeBPS < 0 ||
		s.ProviderReportedFeeBPS > 10000 ||
		s.ReconciliationPendingCount < 0 ||
		s.ReconciliationMismatchCount < 0 ||
		len(s.EvidenceRefs) == 0 ||
		s.ObservedAt.IsZero() {
		return ErrInvalidHealthSnapshot
	}
	switch s.Health {
	case HealthHealthy, HealthDegraded, HealthUnavailable:
	default:
		return ErrInvalidHealthSnapshot
	}
	switch s.Guardrail {
	case GuardrailAllowNewAttempts, GuardrailBlockNewAttempts:
	default:
		return ErrInvalidHealthSnapshot
	}
	return nil
}

func (s ProviderHealthSnapshot) AllowsNewAttempts() bool {
	return s.Validate() == nil &&
		s.Guardrail == GuardrailAllowNewAttempts &&
		s.Health != HealthUnavailable
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func methodsEqual(a, b []MethodCode) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func railsEqual(a, b []RailCode) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
