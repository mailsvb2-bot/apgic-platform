package payments

import (
	"errors"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/audit"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/authz"
)

type collectingAuditAppender struct {
	records []audit.Record
	err     error
}

func (a *collectingAuditAppender) Append(record audit.Record) error {
	if a.err != nil {
		return a.err
	}
	a.records = append(a.records, record)
	return nil
}

func governanceConfig(t0 time.Time) ProviderConfigSnapshot {
	return ProviderConfigSnapshot{
		ProviderID: "provider-a", ConfigVersion: "cfg-v1", Status: ProviderActive,
		Priority: 10, RoutingWeightBPS: 5000,
		Jurisdictions: []string{"RU"}, Currencies: []string{"RUB"},
		Methods: []MethodCode{MethodBankCard}, Rails: []RailCode{RailAPGICPaymentProvider},
		CredentialVersionRef: "secret-version/payment-a/v1", EffectiveFrom: t0,
	}
}

func governanceAuth(t0 time.Time, auditID string, stepUp *time.Time) authz.Input {
	return authz.Input{
		Principal: authz.Principal{
			ID: "admin-1", TenantID: "platform",
			Permissions: map[string]struct{}{"payment.provider.manage": {}},
			StepUpAt: stepUp,
		},
		Resource: authz.ResourceRef{ID: "payments", TenantID: "platform"},
		Now: t0, MaxStepUpAge: 10 * time.Minute,
		CorrelationID: "corr-governance-1", AuditRecordID: auditID,
	}
}

func TestPaymentProviderChangeRequiresHighRiskStepUp(t *testing.T) {
	t0 := time.Now().UTC()
	appender := &collectingAuditAppender{}
	control := ControlPlane{
		PolicyVersion: "payment-governance-v1",
		Authorizer: authz.Evaluator{PolicyVersion: "auth-v1", Appender: appender},
		Appender: appender,
	}
	current := governanceConfig(t0.Add(-time.Hour))
	next := current
	next.ConfigVersion = "cfg-v2"
	next.Status = ProviderPaused
	next.EffectiveFrom = t0

	_, err := control.PlanChange(
		governanceAuth(t0, "auth-audit-1", nil),
		current,
		AdminChangeRequest{
			Action: AdminPause, ActorID: "admin-1", Reason: "provider incident",
			ConfigAuditRecordID: "config-audit-1", NewConfig: next, Now: t0,
		},
	)
	if !errors.Is(err, ErrAdminChangeNotAuthorized) {
		t.Fatalf("missing step-up must block provider change, got %v", err)
	}
	if len(appender.records) != 1 {
		t.Fatalf("authorization decision must be audited even when denied, got %d records", len(appender.records))
	}
}

func TestAuthorizedProviderChangePreservesOldAndNewAuditSnapshots(t *testing.T) {
	t0 := time.Now().UTC()
	stepUp := t0.Add(-time.Minute)
	appender := &collectingAuditAppender{}
	control := ControlPlane{
		PolicyVersion: "payment-governance-v1",
		Authorizer: authz.Evaluator{PolicyVersion: "auth-v1", Appender: appender},
		Appender: appender,
	}
	current := governanceConfig(t0.Add(-time.Hour))
	next := current
	next.ConfigVersion = "cfg-v2"
	next.Priority = 5
	next.RoutingWeightBPS = 7000
	next.EffectiveFrom = t0.Add(time.Minute)

	got, err := control.PlanChange(
		governanceAuth(t0, "auth-audit-2", &stepUp),
		current,
		AdminChangeRequest{
			Action: AdminPrioritize, ActorID: "admin-1", Reason: "approved routing change",
			ConfigAuditRecordID: "config-audit-2", NewConfig: next, Now: t0,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigVersion != "cfg-v2" || len(appender.records) != 2 {
		t.Fatalf("expected authorization + config audit, got %#v records=%d", got, len(appender.records))
	}
	changeAudit := appender.records[1]
	if len(changeAudit.OldState) == 0 || len(changeAudit.NewState) == 0 ||
		changeAudit.ActorID != "admin-1" ||
		changeAudit.Reason != "approved routing change" {
		t.Fatalf("change audit lost old/new/actor/reason evidence: %#v", changeAudit)
	}
}

func TestCredentialRotationCarriesReferenceNotSecret(t *testing.T) {
	t0 := time.Now().UTC()
	current := governanceConfig(t0.Add(-time.Hour))
	next := current
	next.ConfigVersion = "cfg-v2"
	next.CredentialVersionRef = "secret-version/payment-a/v2"
	next.EffectiveFrom = t0
	if !actionMatchesChange(AdminRotateCredential, current, next) {
		t.Fatal("credential reference rotation must be represented as a versioned config change")
	}
}

func TestHealthGuardrailCanBlockNewAttemptsWithoutChangingHistoricalRouting(t *testing.T) {
	t0 := time.Now().UTC()
	snapshot := ProviderHealthSnapshot{
		ProviderID: "provider-a", ConfigVersion: "cfg-v2", Health: HealthDegraded,
		ConversionRateBPS: 8700, LatencyP95MS: 1800, ProviderReportedFeeBPS: 250,
		ReconciliationPendingCount: 3, ReconciliationMismatchCount: 1,
		Guardrail: GuardrailBlockNewAttempts,
		EvidenceRefs: []string{"metrics/provider-a/window-1","reconciliation/run-1"},
		ObservedAt: t0,
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	if snapshot.AllowsNewAttempts() {
		t.Fatal("blocking health snapshot must stop new payment attempts")
	}
}
