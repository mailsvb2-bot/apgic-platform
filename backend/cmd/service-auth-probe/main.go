package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/connector"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/security"
)

type probeProvider struct{ calls int }

func (p *probeProvider) Kind() string { return "staging-probe" }
func (p *probeProvider) Capabilities() []connector.CapabilityClass {
	return []connector.CapabilityClass{connector.CapabilityNotification}
}
func (p *probeProvider) Execute(context.Context, connector.CapabilityClass, connector.Request) (connector.Result, error) {
	p.calls++
	return connector.Result{ProviderReference: "staging-probe", Outcome: connector.OutcomeSuccess}, nil
}

type evidence struct {
	SchemaVersion             string `json:"schema_version"`
	EvidenceType              string `json:"evidence_type"`
	CandidateSHA              string `json:"candidate_sha"`
	ObservedAt                string `json:"observed_at"`
	ActiveCredentialAccepted  bool   `json:"active_credential_accepted"`
	WrongSecretDenied         bool   `json:"wrong_secret_denied"`
	RevokedCredentialDenied   bool   `json:"revoked_credential_denied"`
	ExpiredCredentialDenied   bool   `json:"expired_credential_denied"`
	ExactScopeExecutionPassed bool   `json:"exact_scope_execution_passed"`
	WrongScopeDenied          bool   `json:"wrong_scope_denied"`
	ProviderCalls             int    `json:"provider_calls"`
	SecretMaterialEmitted     bool   `json:"secret_material_emitted"`
}

func secret() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func runProbe(now time.Time, candidateSHA string) (evidence, error) {
	now = now.UTC()
	scope := connector.ExecuteScopeFor(connector.CapabilityNotification)
	principal, err := security.NewServicePrincipal("staging-service-auth-probe", []string{scope})
	if err != nil {
		return evidence{}, err
	}
	activeSecret, err := secret()
	if err != nil {
		return evidence{}, err
	}
	credential, err := security.NewServiceCredential(principal.ID, "staging-v1", activeSecret, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		return evidence{}, err
	}
	authenticated, activeErr := credential.Authenticate(principal, activeSecret, now)
	_, wrongErr := credential.Authenticate(principal, "wrong-"+activeSecret, now)

	revoked, err := credential.Revoke(now)
	if err != nil {
		return evidence{}, err
	}
	_, revokedErr := revoked.Authenticate(principal, activeSecret, now)

	expiredSecret, err := secret()
	if err != nil {
		return evidence{}, err
	}
	expired, err := security.NewServiceCredential(principal.ID, "expired-v1", expiredSecret, now.Add(-2*time.Hour), now.Add(-time.Hour))
	if err != nil {
		return evidence{}, err
	}
	_, expiredErr := expired.Authenticate(principal, expiredSecret, now)

	instance, err := connector.NewInstance("staging-notification", connector.CapabilityNotification, "staging-probe", "staging://probe")
	if err != nil {
		return evidence{}, err
	}
	instance.Status = connector.StatusActive
	provider := &probeProvider{}
	_, execErr := connector.Execute(
		context.Background(),
		instance,
		provider,
		authenticated,
		connector.Request{IdempotencyKey: "sec001-staging-probe", SubjectID: "sec001", Payload: json.RawMessage(`{"probe":true}`)},
		connector.ExecutionPolicy{Now: func() time.Time { return now }},
	)

	wrongPrincipal, err := security.NewServicePrincipal("staging-service-auth-wrong-scope", []string{connector.ExecuteScopeFor(connector.CapabilityCalendar)})
	if err != nil {
		return evidence{}, err
	}
	wrongScopeSecret, err := secret()
	if err != nil {
		return evidence{}, err
	}
	wrongCredential, err := security.NewServiceCredential(wrongPrincipal.ID, "staging-v1", wrongScopeSecret, now.Add(-time.Minute), now.Add(time.Hour))
	if err != nil {
		return evidence{}, err
	}
	wrongAuthenticated, err := wrongCredential.Authenticate(wrongPrincipal, wrongScopeSecret, now)
	if err != nil {
		return evidence{}, err
	}
	_, wrongScopeErr := connector.Execute(
		context.Background(),
		instance,
		provider,
		wrongAuthenticated,
		connector.Request{IdempotencyKey: "sec001-wrong-scope", SubjectID: "sec001"},
		connector.ExecutionPolicy{Now: func() time.Time { return now }},
	)

	out := evidence{
		SchemaVersion:             "sec001-staging-service-auth-v1",
		EvidenceType:              "STAGING_SERVICE_AUTH_PROOF",
		CandidateSHA:              candidateSHA,
		ObservedAt:                now.Format(time.RFC3339),
		ActiveCredentialAccepted:  activeErr == nil,
		WrongSecretDenied:         errors.Is(wrongErr, security.ErrServiceSecretMismatch),
		RevokedCredentialDenied:   errors.Is(revokedErr, security.ErrServiceCredentialDenied),
		ExpiredCredentialDenied:   errors.Is(expiredErr, security.ErrServiceCredentialDenied),
		ExactScopeExecutionPassed: execErr == nil,
		WrongScopeDenied:          errors.Is(wrongScopeErr, connector.ErrConnectorScopeDenied),
		ProviderCalls:             provider.calls,
		SecretMaterialEmitted:     false,
	}
	if !out.ActiveCredentialAccepted || !out.WrongSecretDenied || !out.RevokedCredentialDenied ||
		!out.ExpiredCredentialDenied || !out.ExactScopeExecutionPassed || !out.WrongScopeDenied ||
		out.ProviderCalls != 1 {
		return out, fmt.Errorf("service authentication staging probe failed")
	}
	return out, nil
}

func main() {
	out, err := runProbe(time.Now(), os.Getenv("APGIC_COMMIT_SHA"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
