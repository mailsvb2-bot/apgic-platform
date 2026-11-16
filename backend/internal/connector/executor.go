package connector

import (
	"context"
	"errors"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/security"
)

var (
	ErrConnectorUnavailable      = errors.New("connector is not available for execution")
	ErrConnectorDegraded         = errors.New("connector is degraded and degraded execution is not allowed")
	ErrConnectorScopeDenied      = errors.New("service principal lacks connector execute scope")
	ErrConnectorCredentialDenied = errors.New("service principal credential is not active")
	ErrProviderMismatch          = errors.New("provider does not match connector instance")
	ErrCapabilityMismatch        = errors.New("provider does not expose connector capability")
	ErrTypedCapabilityRequired   = errors.New("CDN_PROVIDER must use the versioned playback capability")
	ErrInvalidProviderResult     = errors.New("provider result violates connector contract")
)

type ExecutionPolicy struct {
	AllowDegraded bool
	Now           func() time.Time
}

func executionNow(policy ExecutionPolicy) time.Time {
	now := time.Now().UTC()
	if policy.Now != nil {
		now = policy.Now().UTC()
	}
	return now
}

func authorizeConnectorExecution(
	ctx context.Context,
	instance Instance,
	provider Provider,
	principal security.AuthenticatedServicePrincipal,
	policy ExecutionPolicy,
) error {
	if ctx == nil || ctx.Err() != nil || !instance.Capability.Valid() || !instance.Status.Valid() ||
		instance.ExecuteScope != ExecuteScopeFor(instance.Capability) {
		return ErrInvalidConnector
	}
	now := executionNow(policy)
	if !principal.ValidAt(now) {
		return ErrConnectorCredentialDenied
	}
	if !principal.HasScope(instance.ExecuteScope) {
		return ErrConnectorScopeDenied
	}

	switch instance.Status {
	case StatusConfiguring, StatusDisabled:
		return ErrConnectorUnavailable
	case StatusDegraded:
		if !policy.AllowDegraded {
			return ErrConnectorDegraded
		}
	case StatusActive:
		// ready
	default:
		return ErrConnectorUnavailable
	}

	if provider == nil || provider.Kind() != instance.ProviderKind {
		return ErrProviderMismatch
	}
	if !providerHasCapability(provider, instance.Capability) {
		return ErrCapabilityMismatch
	}
	return nil
}

// Execute is reserved for capabilities whose contract is Provider.Execute.
// CDN playback is deliberately excluded: callers must use the typed grant and
// attestation methods below so a generic JSON result cannot certify playback.
func Execute(
	ctx context.Context,
	instance Instance,
	provider Provider,
	principal security.AuthenticatedServicePrincipal,
	request Request,
	policy ExecutionPolicy,
) (Result, error) {
	if err := request.Validate(); err != nil {
		return Result{}, err
	}
	if err := authorizeConnectorExecution(ctx, instance, provider, principal, policy); err != nil {
		return Result{}, err
	}
	if instance.Capability == CapabilityCDN {
		if err := ValidateCDNPlaybackProvider(provider); err != nil {
			return Result{}, errors.Join(ErrCapabilityMismatch, err)
		}
		return Result{}, ErrTypedCapabilityRequired
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

// authorizeConnectorRevocation deliberately permits a disabled or degraded
// connector: emergency session invalidation must remain possible after new
// playback has been stopped. It requires a distinct, short-lived revoke scope.
func authorizeConnectorRevocation(
	ctx context.Context,
	instance Instance,
	provider Provider,
	principal security.AuthenticatedServicePrincipal,
	policy ExecutionPolicy,
) error {
	if ctx == nil || ctx.Err() != nil || !instance.Capability.Valid() ||
		!instance.Status.Valid() || instance.ExecuteScope != ExecuteScopeFor(instance.Capability) {
		return ErrInvalidConnector
	}
	if !principal.ValidAt(executionNow(policy)) {
		return ErrConnectorCredentialDenied
	}
	if !principal.HasScope(RevokeScopeFor(instance.Capability)) {
		return ErrConnectorScopeDenied
	}
	if provider == nil || provider.Kind() != instance.ProviderKind {
		return ErrProviderMismatch
	}
	if !providerHasCapability(provider, instance.Capability) {
		return ErrCapabilityMismatch
	}
	return nil
}

// RevokeCDNPlaybackGrant synchronously asks the owning provider to invalidate
// the exact playback session. Providers must make this idempotent and close
// already-open media streams, not merely deny future grant creation.
func RevokeCDNPlaybackGrant(
	ctx context.Context,
	instance Instance,
	provider Provider,
	principal security.AuthenticatedServicePrincipal,
	grant CDNPlaybackGrant,
	policy ExecutionPolicy,
) error {
	if grant.Validate() != nil || grant.ProviderInstanceID != instance.ID ||
		grant.ProviderKind != instance.ProviderKind || instance.Capability != CapabilityCDN {
		return ErrInvalidCDNPlaybackGrant
	}
	if err := authorizeConnectorRevocation(ctx, instance, provider, principal, policy); err != nil {
		return err
	}
	if err := ValidateCDNPlaybackProvider(provider); err != nil {
		return errors.Join(ErrCapabilityMismatch, err)
	}
	return provider.(CDNPlaybackCapability).RevokePlaybackGrant(ctx, grant)
}

// IssueCDNPlaybackGrant applies the same connector status, service-principal
// scope and provider identity checks as Execute before calling the typed CDN
// contract. A provider cannot grant a different learner, lesson or entitlement.
func IssueCDNPlaybackGrant(
	ctx context.Context,
	instance Instance,
	provider Provider,
	principal security.AuthenticatedServicePrincipal,
	request CDNPlaybackRequest,
	policy ExecutionPolicy,
) (CDNPlaybackGrant, error) {
	if err := request.Validate(); err != nil {
		return CDNPlaybackGrant{}, err
	}
	if request.ProviderInstanceID != instance.ID {
		return CDNPlaybackGrant{}, ErrInvalidCDNPlaybackRequest
	}
	if err := authorizeConnectorExecution(ctx, instance, provider, principal, policy); err != nil {
		return CDNPlaybackGrant{}, err
	}
	if instance.Capability != CapabilityCDN {
		return CDNPlaybackGrant{}, ErrCapabilityMismatch
	}
	if err := ValidateCDNPlaybackProvider(provider); err != nil {
		return CDNPlaybackGrant{}, errors.Join(ErrCapabilityMismatch, err)
	}

	grant, err := provider.(CDNPlaybackCapability).IssuePlaybackGrant(ctx, request)
	if err != nil {
		return CDNPlaybackGrant{}, err
	}
	now := executionNow(policy)
	if grant.Validate() != nil || grant.ProviderKind != instance.ProviderKind ||
		grant.ProviderInstanceID != instance.ID ||
		grant.Binding != request.Binding || grant.IssuedAt.After(now.Add(CDNPlaybackClockSkew)) ||
		!grant.ExpiresAt.After(now) {
		return CDNPlaybackGrant{}, ErrInvalidCDNPlaybackGrant
	}
	return grant, nil
}

// OpenCDNPlaybackStream is the only connector-executor path for provider bytes.
// It enforces the exact active connector, principal scope, grant lifetime and
// client range before provider I/O, then validates the provider's bounded
// representation. Invalid or cancelled streams are always closed here.
func OpenCDNPlaybackStream(
	ctx context.Context,
	instance Instance,
	provider Provider,
	principal security.AuthenticatedServicePrincipal,
	grant CDNPlaybackGrant,
	requested *CDNPlaybackByteRange,
	policy ExecutionPolicy,
) (CDNPlaybackStream, error) {
	if grant.Validate() != nil || grant.ProviderInstanceID != instance.ID ||
		grant.ProviderKind != instance.ProviderKind || instance.Capability != CapabilityCDN {
		return CDNPlaybackStream{}, ErrInvalidCDNPlaybackGrant
	}
	now := executionNow(policy)
	if !grant.ExpiresAt.After(now) || grant.IssuedAt.After(now.Add(CDNPlaybackClockSkew)) {
		return CDNPlaybackStream{}, ErrInvalidCDNPlaybackGrant
	}
	if err := authorizeConnectorExecution(ctx, instance, provider, principal, policy); err != nil {
		return CDNPlaybackStream{}, err
	}
	if err := ValidateCDNPlaybackProvider(provider); err != nil {
		return CDNPlaybackStream{}, errors.Join(ErrCapabilityMismatch, err)
	}
	if requested != nil {
		if err := requested.Validate(grant.Binding.ContentLength); err != nil {
			return CDNPlaybackStream{}, err
		}
	}
	var providerRange *CDNPlaybackByteRange
	if requested != nil {
		copyOfRange := *requested
		providerRange = &copyOfRange
	}
	stream, err := provider.(CDNPlaybackCapability).OpenPlaybackStream(ctx, grant, providerRange)
	if err != nil {
		if stream.Body != nil {
			_ = stream.Body.Close()
		}
		return CDNPlaybackStream{}, err
	}
	if ctx.Err() != nil {
		if stream.Body != nil {
			_ = stream.Body.Close()
		}
		return CDNPlaybackStream{}, ctx.Err()
	}
	if err := ValidateCDNPlaybackStream(grant.Binding, requested, stream); err != nil {
		if stream.Body != nil {
			_ = stream.Body.Close()
		}
		return CDNPlaybackStream{}, err
	}
	return stream, nil
}

// VerifyCDNPlaybackEvidence verifies evidence through the configured CDN
// connector while preserving the exact input payload. This prevents an
// adapter from returning a different, previously verified attestation.
func VerifyCDNPlaybackEvidence(
	ctx context.Context,
	instance Instance,
	provider Provider,
	principal security.AuthenticatedServicePrincipal,
	grant CDNPlaybackGrant,
	attestation CDNPlaybackAttestation,
	policy ExecutionPolicy,
) (VerifiedCDNPlaybackAttestation, error) {
	if grant.Validate() != nil || grant.ProviderInstanceID != instance.ID ||
		attestation.ContractVersion != CDNPlaybackContractVersion ||
		attestation.ProviderInstanceID != instance.ID ||
		attestation.ProviderKind != grant.ProviderKind || attestation.SessionID != grant.SessionID ||
		attestation.Binding != grant.Binding {
		return VerifiedCDNPlaybackAttestation{}, ErrInvalidCDNPlaybackAttestation
	}
	if err := authorizeConnectorExecution(ctx, instance, provider, principal, policy); err != nil {
		return VerifiedCDNPlaybackAttestation{}, err
	}
	if instance.Capability != CapabilityCDN || grant.ProviderKind != instance.ProviderKind {
		return VerifiedCDNPlaybackAttestation{}, ErrCapabilityMismatch
	}
	if err := ValidateCDNPlaybackProvider(provider); err != nil {
		return VerifiedCDNPlaybackAttestation{}, errors.Join(ErrCapabilityMismatch, err)
	}

	verified, err := provider.(CDNPlaybackCapability).VerifyPlaybackAttestation(ctx, grant, attestation)
	if err != nil {
		return VerifiedCDNPlaybackAttestation{}, err
	}
	if !verified.Valid() || !cdnPlaybackAttestationMatches(verified.attestation, attestation) {
		return VerifiedCDNPlaybackAttestation{}, ErrInvalidCDNPlaybackAttestation
	}
	return verified, nil
}

func providerHasCapability(provider Provider, capability CapabilityClass) bool {
	for _, candidate := range provider.Capabilities() {
		if candidate == capability {
			return true
		}
	}
	return false
}
