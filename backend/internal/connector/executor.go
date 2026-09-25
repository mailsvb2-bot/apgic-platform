package connector

import (
	"context"
	"errors"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/security"
)

var (
	ErrConnectorUnavailable  = errors.New("connector is not available for execution")
	ErrConnectorDegraded     = errors.New("connector is degraded and degraded execution is not allowed")
	ErrConnectorScopeDenied  = errors.New("service principal lacks connector execute scope")
	ErrProviderMismatch      = errors.New("provider does not match connector instance")
	ErrCapabilityMismatch    = errors.New("provider does not expose connector capability")
	ErrInvalidProviderResult = errors.New("provider result violates connector contract")
)

type ExecutionPolicy struct {
	AllowDegraded bool
}

func Execute(
	ctx context.Context,
	instance Instance,
	provider Provider,
	principal security.ServicePrincipal,
	request Request,
	policy ExecutionPolicy,
) (Result, error) {
	if ctx == nil || !instance.Capability.Valid() || !instance.Status.Valid() ||
		instance.ExecuteScope != ExecuteScopeFor(instance.Capability) {
		return Result{}, ErrInvalidConnector
	}
	if err := request.Validate(); err != nil {
		return Result{}, err
	}
	if !principal.HasScope(instance.ExecuteScope) {
		return Result{}, ErrConnectorScopeDenied
	}

	switch instance.Status {
	case StatusConfiguring, StatusDisabled:
		return Result{}, ErrConnectorUnavailable
	case StatusDegraded:
		if !policy.AllowDegraded {
			return Result{}, ErrConnectorDegraded
		}
	case StatusActive:
		// ready
	default:
		return Result{}, ErrConnectorUnavailable
	}

	if provider == nil || provider.Kind() != instance.ProviderKind {
		return Result{}, ErrProviderMismatch
	}
	if !providerHasCapability(provider, instance.Capability) {
		return Result{}, ErrCapabilityMismatch
	}

	result, err := provider.Execute(ctx, instance.Capability, request)
	if err != nil {
		return Result{}, err
	}
	if err := result.Validate(); err != nil {
		return Result{}, errors.Join(ErrInvalidProviderResult, err)
	}
	return result, nil
}

func providerHasCapability(provider Provider, capability CapabilityClass) bool {
	for _, candidate := range provider.Capabilities() {
		if candidate == capability {
			return true
		}
	}
	return false
}
