package payments

import (
	"errors"
	"sort"
	"strings"
)

const ExternalExecutionOwner = "EXTERNAL_PROVIDER"

type ProviderStatus string

const (
	ProviderActive   ProviderStatus = "ACTIVE"
	ProviderDegraded ProviderStatus = "DEGRADED"
	ProviderPaused   ProviderStatus = "PAUSED"
	ProviderDisabled ProviderStatus = "DISABLED"
)

type ProviderHealth string

const (
	HealthHealthy     ProviderHealth = "HEALTHY"
	HealthDegraded    ProviderHealth = "DEGRADED"
	HealthUnavailable ProviderHealth = "UNAVAILABLE"
)

type AttemptState string

const (
	AttemptCreated        AttemptState = "CREATED"
	AttemptSent           AttemptState = "SENT"
	AttemptPending        AttemptState = "PENDING"
	AttemptSucceeded      AttemptState = "SUCCEEDED"
	AttemptFailedTerminal AttemptState = "FAILED_TERMINAL"
	AttemptAmbiguous      AttemptState = "AMBIGUOUS"
)

const (
	ReasonRouteSelected             = "PAY_ROUTE_SELECTED"
	ReasonNoEligibleProvider        = "PAY_NO_ELIGIBLE_PROVIDER"
	ReasonProviderInactive          = "PAY_PROVIDER_INACTIVE"
	ReasonProviderUncertified       = "PAY_PROVIDER_UNCERTIFIED"
	ReasonProviderUnavailable       = "PAY_PROVIDER_UNAVAILABLE"
	ReasonExternalExecutionRequired = "PAY_EXTERNAL_EXECUTION_REQUIRED"
	ReasonMethodUnsupported         = "PAY_METHOD_UNSUPPORTED"
	ReasonRailUnsupported           = "PAY_RAIL_UNSUPPORTED"
	ReasonCurrencyUnsupported       = "PAY_CURRENCY_UNSUPPORTED"
	ReasonJurisdictionUnsupported   = "PAY_JURISDICTION_UNSUPPORTED"
)

var (
	ErrInvalidManifest      = errors.New("invalid payment provider manifest")
	ErrInvalidRoutingPolicy = errors.New("invalid payment routing policy")
	ErrInvalidTransaction   = errors.New("invalid payment transaction context")
	ErrNoEligibleProvider   = errors.New("no eligible payment provider")
	ErrInvalidInstruction   = errors.New("invalid payment instruction")
)

type ProviderManifest struct {
	Version                   string
	ProviderID                ProviderID
	Certified                 bool
	CertificationEvidenceRefs []string
	ExecutionOwner            string
	Methods                   []MethodCode
	Rails                     []RailCode
	Currencies                []string
	Jurisdictions             []string
	RegulatedCapabilities     []string
}

func (m ProviderManifest) Validate() error {
	if strings.TrimSpace(m.Version) == "" ||
		strings.TrimSpace(string(m.ProviderID)) == "" ||
		!m.Certified ||
		len(m.CertificationEvidenceRefs) == 0 ||
		m.ExecutionOwner != ExternalExecutionOwner ||
		len(m.Methods) == 0 ||
		len(m.Rails) == 0 ||
		len(m.Currencies) == 0 ||
		len(m.Jurisdictions) == 0 {
		return ErrInvalidManifest
	}
	for _, currency := range m.Currencies {
		if len(currency) != 3 || currency != strings.ToUpper(currency) {
			return ErrInvalidManifest
		}
	}
	return nil
}

type ProviderInstance struct {
	Manifest ProviderManifest
	Status   ProviderStatus
	Health   ProviderHealth
	Priority int
}

type TransactionContext struct {
	OrderID      string
	AmountMinor  int64
	Currency     string
	Jurisdiction string
	Method       MethodCode
	Rail         RailCode
}

func (t TransactionContext) Validate() error {
	if strings.TrimSpace(t.OrderID) == "" ||
		t.AmountMinor <= 0 ||
		len(t.Currency) != 3 ||
		t.Currency != strings.ToUpper(t.Currency) ||
		strings.TrimSpace(t.Jurisdiction) == "" ||
		t.Method == "" ||
		t.Rail == "" {
		return ErrInvalidTransaction
	}
	return nil
}

type RoutingPolicy struct {
	Version          string
	OrderedProviders []ProviderID
}

type CandidateDecision struct {
	ProviderID ProviderID
	Eligible   bool
	ReasonCode string
}

type RoutingDecision struct {
	PolicyVersion string
	ProviderID    ProviderID
	Candidates    []CandidateDecision
	ReasonCode    string
}

func SelectProvider(tx TransactionContext, providers []ProviderInstance, policy RoutingPolicy) (RoutingDecision, error) {
	if err := tx.Validate(); err != nil {
		return RoutingDecision{}, err
	}
	if strings.TrimSpace(policy.Version) == "" || len(policy.OrderedProviders) == 0 {
		return RoutingDecision{}, ErrInvalidRoutingPolicy
	}

	byID := make(map[ProviderID]ProviderInstance, len(providers))
	for _, provider := range providers {
		if err := provider.Manifest.Validate(); err != nil {
			continue
		}
		byID[provider.Manifest.ProviderID] = provider
	}

	seen := make(map[ProviderID]struct{}, len(policy.OrderedProviders))
	decision := RoutingDecision{PolicyVersion: policy.Version, ReasonCode: ReasonNoEligibleProvider}
	for _, providerID := range policy.OrderedProviders {
		if _, duplicate := seen[providerID]; duplicate {
			return RoutingDecision{}, ErrInvalidRoutingPolicy
		}
		seen[providerID] = struct{}{}

		provider, ok := byID[providerID]
		if !ok {
			decision.Candidates = append(decision.Candidates, CandidateDecision{
				ProviderID: providerID, ReasonCode: ReasonProviderUncertified,
			})
			continue
		}
		reason := eligibilityReason(tx, provider)
		eligible := reason == ReasonRouteSelected
		decision.Candidates = append(decision.Candidates, CandidateDecision{
			ProviderID: providerID, Eligible: eligible, ReasonCode: reason,
		})
		if eligible {
			decision.ProviderID = providerID
			decision.ReasonCode = ReasonRouteSelected
			return decision, nil
		}
	}
	return decision, ErrNoEligibleProvider
}

func eligibilityReason(tx TransactionContext, provider ProviderInstance) string {
	if provider.Status != ProviderActive && provider.Status != ProviderDegraded {
		return ReasonProviderInactive
	}
	if !provider.Manifest.Certified {
		return ReasonProviderUncertified
	}
	if provider.Health == HealthUnavailable {
		return ReasonProviderUnavailable
	}
	if provider.Manifest.ExecutionOwner != ExternalExecutionOwner {
		return ReasonExternalExecutionRequired
	}
	if !containsMethod(provider.Manifest.Methods, tx.Method) {
		return ReasonMethodUnsupported
	}
	if !containsRail(provider.Manifest.Rails, tx.Rail) {
		return ReasonRailUnsupported
	}
	if !containsString(provider.Manifest.Currencies, tx.Currency) {
		return ReasonCurrencyUnsupported
	}
	if !containsString(provider.Manifest.Jurisdictions, tx.Jurisdiction) {
		return ReasonJurisdictionUnsupported
	}
	return ReasonRouteSelected
}

type CatalogContext struct {
	Currency     string
	Jurisdiction string
	Rail         RailCode
}

func EligibleMethods(ctx CatalogContext, providers []ProviderInstance) []MethodCode {
	seen := map[MethodCode]struct{}{}
	for _, provider := range providers {
		if provider.Manifest.Validate() != nil ||
			(provider.Status != ProviderActive && provider.Status != ProviderDegraded) ||
			provider.Health == HealthUnavailable ||
			provider.Manifest.ExecutionOwner != ExternalExecutionOwner ||
			!containsString(provider.Manifest.Currencies, ctx.Currency) ||
			!containsString(provider.Manifest.Jurisdictions, ctx.Jurisdiction) ||
			!containsRail(provider.Manifest.Rails, ctx.Rail) {
			continue
		}
		for _, method := range provider.Manifest.Methods {
			seen[method] = struct{}{}
		}
	}
	out := make([]MethodCode, 0, len(seen))
	for method := range seen {
		out = append(out, method)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func CanFallback(state AttemptState) bool {
	return state == AttemptCreated || state == AttemptFailedTerminal
}

type PaymentInstruction struct {
	OrderID          string
	ProviderID       ProviderID
	Method           MethodCode
	Rail             RailCode
	AmountMinor      int64
	Currency         string
	IdempotencyKey   string
	RoutingPolicyRef string
}

func (i PaymentInstruction) Validate() error {
	if strings.TrimSpace(i.OrderID) == "" ||
		strings.TrimSpace(string(i.ProviderID)) == "" ||
		i.Method == "" ||
		i.Rail == "" ||
		i.AmountMinor <= 0 ||
		len(i.Currency) != 3 ||
		i.Currency != strings.ToUpper(i.Currency) ||
		strings.TrimSpace(i.IdempotencyKey) == "" ||
		strings.TrimSpace(i.RoutingPolicyRef) == "" {
		return ErrInvalidInstruction
	}
	return nil
}

func containsMethod(values []MethodCode, needle MethodCode) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func containsRail(values []RailCode, needle RailCode) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
