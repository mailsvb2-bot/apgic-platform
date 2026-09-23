package audit

import "testing"

func TestHighRiskAuditRequiresReasonAndPolicyVersion(t *testing.T) {
	_, err := New(Record{ID: "a-1", ActorID: "operator", Action: "override", Scope: "org-1"})
	if err != ErrInvalidRecord {
		t.Fatalf("expected validation error, got %v", err)
	}
}
