package authz

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/audit"
)

type memoryAuditAppender struct {
	records []audit.Record
	err     error
}

func (m *memoryAuditAppender) Append(record audit.Record) error {
	if m.err != nil {
		return m.err
	}
	m.records = append(m.records, record)
	return nil
}

func auditableInput(now time.Time) Input {
	in := normalInput(now)
	in.CorrelationID = "corr-authz-1"
	in.AuditRecordID = "audit-authz-1"
	return in
}

func TestCrossTenantDenialPersistsReasonedAuditEvidence(t *testing.T) {
	now := time.Now().UTC()
	in := auditableInput(now)
	in.Resource = ResourceRef{ID: "known-private-uuid", TenantID: "org-b"}

	appender := &memoryAuditAppender{}
	evaluator := Evaluator{
		PolicyVersion: "authz-policy-v1",
		Appender:      appender,
	}

	result, err := evaluator.Authorize(in)
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != Deny || result.ReasonCode != "AUTH_CROSS_TENANT_DENY" {
		t.Fatalf("unexpected decision: %#v", result)
	}
	if len(appender.records) != 1 {
		t.Fatalf("audit record count = %d, want 1", len(appender.records))
	}

	record := appender.records[0]
	if record.ActorID != "alice" ||
		record.Action != "authorization.decision" ||
		record.Scope != "org-a" ||
		record.ResourceRef != "tenant/org-b/resource/known-private-uuid" ||
		record.Reason != "AUTH_CROSS_TENANT_DENY" ||
		record.PolicyVersion != "authz-policy-v1" ||
		record.CorrelationID != "corr-authz-1" {
		t.Fatalf("unexpected audit record: %#v", record)
	}
	if len(record.NewState) == 0 {
		t.Fatal("authorization decision snapshot is missing")
	}
}

func TestStepUpDecisionIsAudited(t *testing.T) {
	now := time.Now().UTC()
	in := auditableInput(now)
	in.Principal.Permissions = map[string]struct{}{"organization.transfer_ownership": {}}
	in.Action = "organization.transfer_ownership"
	in.Risk = RiskHigh
	in.Principal.SessionRef = "session:test"

	appender := &memoryAuditAppender{}
	evaluator := Evaluator{PolicyVersion: "authz-policy-v1", Appender: appender}

	result, err := evaluator.Authorize(in)
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != StepUpRequired || result.ReasonCode != "AUTH_STEP_UP_REQUIRED" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(appender.records) != 1 || appender.records[0].Reason != "AUTH_STEP_UP_REQUIRED" {
		t.Fatalf("step-up evidence missing: %#v", appender.records)
	}
	var snapshot decisionSnapshot
	if err := json.Unmarshal(appender.records[0].NewState, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.SecurityEvidence == nil ||
		snapshot.SecurityEvidence.PrincipalID != "alice" ||
		snapshot.SecurityEvidence.SessionRef != "session:test" ||
		snapshot.SecurityEvidence.Decision != StepUpRequired ||
		snapshot.SecurityEvidence.ReasonCode != "AUTH_STEP_UP_REQUIRED" ||
		snapshot.SecurityEvidence.PolicyVersion != "authz-policy-v1" {
		t.Fatalf("high-risk security evidence missing: %#v", snapshot.SecurityEvidence)
	}
}

func TestAuditFailureFailsClosedEvenWhenPolicyWouldAllow(t *testing.T) {
	now := time.Now().UTC()
	in := auditableInput(now)
	appender := &memoryAuditAppender{err: errors.New("audit storage unavailable")}
	evaluator := Evaluator{PolicyVersion: "authz-policy-v1", Appender: appender}

	result, err := evaluator.Authorize(in)
	if !errors.Is(err, ErrAuditUnavailable) {
		t.Fatalf("expected ErrAuditUnavailable, got %v", err)
	}
	if result.Decision != Deny || result.ReasonCode != "AUTH_AUDIT_UNAVAILABLE" {
		t.Fatalf("audit outage must fail closed, got %#v", result)
	}
}

func TestMissingAuditMetadataFailsClosed(t *testing.T) {
	in := normalInput(time.Now().UTC())
	evaluator := Evaluator{
		PolicyVersion: "authz-policy-v1",
		Appender:      &memoryAuditAppender{},
	}

	result, err := evaluator.Authorize(in)
	if !errors.Is(err, ErrAuditEvidenceRequired) {
		t.Fatalf("expected ErrAuditEvidenceRequired, got %v", err)
	}
	if result.Decision != Deny || result.ReasonCode != "AUTH_AUDIT_EVIDENCE_REQUIRED" {
		t.Fatalf("missing evidence metadata must fail closed: %#v", result)
	}
}

func TestTenantContextDenialIsAudited(t *testing.T) {
	now := time.Now().UTC()
	in := auditableInput(now)
	in.TenantContextDenied = true

	appender := &memoryAuditAppender{}
	evaluator := Evaluator{PolicyVersion: "authz-policy-v1", Appender: appender}
	result, err := evaluator.Authorize(in)
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != Deny || result.ReasonCode != "AUTH_TENANT_CONTEXT_DENIED" {
		t.Fatalf("unexpected tenant-context result: %#v", result)
	}
	if len(appender.records) != 1 || appender.records[0].Reason != "AUTH_TENANT_CONTEXT_DENIED" {
		t.Fatalf("tenant-context denial evidence missing: %#v", appender.records)
	}
}

func TestAllowedHighRiskAuditCarriesStepUpMethodAndTimestamp(t *testing.T) {
	now := time.Now().UTC()
	stepUpAt := now.Add(-time.Minute)
	in := auditableInput(now)
	in.Principal.Permissions = map[string]struct{}{"organization.transfer_ownership": {}}
	in.Principal.SessionRef = "session:bound"
	in.Principal.StepUpAt = &stepUpAt
	in.Principal.StepUpMethod = "WEBAUTHN"
	in.Action = "organization.transfer_ownership"
	in.Risk = RiskHigh

	appender := &memoryAuditAppender{}
	result, err := (Evaluator{PolicyVersion: "authz-policy-v1", Appender: appender}).Authorize(in)
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != Allow || len(appender.records) != 1 {
		t.Fatalf("unexpected allowed high-risk result=%#v records=%#v", result, appender.records)
	}
	var snapshot decisionSnapshot
	if err := json.Unmarshal(appender.records[0].NewState, &snapshot); err != nil {
		t.Fatal(err)
	}
	evidence := snapshot.SecurityEvidence
	if evidence == nil || evidence.Method != "WEBAUTHN" || evidence.StepUpAt == nil ||
		!evidence.StepUpAt.Equal(stepUpAt) || !evidence.EvaluatedAt.Equal(now) {
		t.Fatalf("step-up method/timestamps not preserved: %#v", evidence)
	}
}
