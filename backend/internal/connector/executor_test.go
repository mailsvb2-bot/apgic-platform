package connector

import (
	"context"
	"errors"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/security"
)

type fakeProvider struct {
	kind         string
	capabilities []CapabilityClass
	result       Result
	err          error
	calls        int
}

func (p *fakeProvider) Kind() string {
	return p.kind
}

func (p *fakeProvider) Capabilities() []CapabilityClass {
	return p.capabilities
}

func (p *fakeProvider) Execute(context.Context, CapabilityClass, Request) (Result, error) {
	p.calls++
	return p.result, p.err
}

func scopedPrincipal(t *testing.T, scope string) security.ServicePrincipal {
	t.Helper()
	principal, err := security.NewServicePrincipal("connector-runtime", []string{scope})
	if err != nil {
		t.Fatal(err)
	}
	return principal
}

func activeNotificationInstance(t *testing.T) Instance {
	t.Helper()
	instance, err := NewInstance(
		"connector-1",
		CapabilityNotification,
		"provider-a",
		"config/provider-a",
	)
	if err != nil {
		t.Fatal(err)
	}
	instance.Status = StatusActive
	return instance
}

func validConnectorRequest() Request {
	return Request{
		IdempotencyKey: "notification/message-1",
		SubjectID:      "message-1",
		Payload:        []byte(`{"message_id":"message-1"}`),
	}
}

func TestScopedExecutorCallsProviderOnlyWithExactConnectorScope(t *testing.T) {
	instance := activeNotificationInstance(t)
	provider := &fakeProvider{
		kind:         "provider-a",
		capabilities: []CapabilityClass{CapabilityNotification},
		result: Result{
			ProviderReference: "external-1",
			Outcome:           OutcomeSuccess,
			RawEvidence:       []byte(`{"accepted":true}`),
		},
	}

	wrongPrincipal := scopedPrincipal(t, "connector:execute:CALENDAR_PROVIDER")
	if _, err := Execute(
		context.Background(),
		instance,
		provider,
		wrongPrincipal,
		validConnectorRequest(),
		ExecutionPolicy{},
	); !errors.Is(err, ErrConnectorScopeDenied) {
		t.Fatalf("expected scope denial, got %v", err)
	}
	if provider.calls != 0 {
		t.Fatal("provider was called before scope authorization")
	}

	rightPrincipal := scopedPrincipal(t, instance.ExecuteScope)
	result, err := Execute(
		context.Background(),
		instance,
		provider,
		rightPrincipal,
		validConnectorRequest(),
		ExecutionPolicy{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeSuccess || provider.calls != 1 {
		t.Fatalf("unexpected result/calls: %#v calls=%d", result, provider.calls)
	}
}

func TestDegradedConnectorRequiresExplicitPolicy(t *testing.T) {
	instance := activeNotificationInstance(t)
	instance.Status = StatusDegraded
	provider := &fakeProvider{
		kind:         "provider-a",
		capabilities: []CapabilityClass{CapabilityNotification},
		result:       Result{Outcome: OutcomeSuccess},
	}
	principal := scopedPrincipal(t, instance.ExecuteScope)

	if _, err := Execute(
		context.Background(),
		instance,
		provider,
		principal,
		validConnectorRequest(),
		ExecutionPolicy{},
	); !errors.Is(err, ErrConnectorDegraded) {
		t.Fatalf("expected degraded denial, got %v", err)
	}

	if _, err := Execute(
		context.Background(),
		instance,
		provider,
		principal,
		validConnectorRequest(),
		ExecutionPolicy{AllowDegraded: true},
	); err != nil {
		t.Fatalf("explicit degraded execution should proceed: %v", err)
	}
}

func TestProviderAndCapabilityMismatchFailBeforeExecution(t *testing.T) {
	instance := activeNotificationInstance(t)
	principal := scopedPrincipal(t, instance.ExecuteScope)

	wrongProvider := &fakeProvider{
		kind:         "provider-b",
		capabilities: []CapabilityClass{CapabilityNotification},
		result:       Result{Outcome: OutcomeSuccess},
	}
	if _, err := Execute(
		context.Background(),
		instance,
		wrongProvider,
		principal,
		validConnectorRequest(),
		ExecutionPolicy{},
	); !errors.Is(err, ErrProviderMismatch) {
		t.Fatalf("expected provider mismatch, got %v", err)
	}

	missingCapability := &fakeProvider{
		kind:         "provider-a",
		capabilities: []CapabilityClass{CapabilityCalendar},
		result:       Result{Outcome: OutcomeSuccess},
	}
	if _, err := Execute(
		context.Background(),
		instance,
		missingCapability,
		principal,
		validConnectorRequest(),
		ExecutionPolicy{},
	); !errors.Is(err, ErrCapabilityMismatch) {
		t.Fatalf("expected capability mismatch, got %v", err)
	}
}

func TestRetryPolicyNeverRetriesAmbiguousOutcome(t *testing.T) {
	if !ShouldRetry(OutcomeRetryableFailure, 1, 3) {
		t.Fatal("safe retryable failure should retry within budget")
	}
	if ShouldRetry(OutcomeAmbiguous, 1, 3) {
		t.Fatal("ambiguous provider outcome must not blind-retry")
	}
	if ShouldRetry(OutcomeRetryableFailure, 3, 3) {
		t.Fatal("retry budget must be enforced")
	}
}
