package authz

import (
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
