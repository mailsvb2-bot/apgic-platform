package audit

import (
	"testing"
	"time"
)

func TestHighRiskAuditRequiresReasonAndPolicyVersion(t *testing.T) {
	_, err := New(Record{
		ID: "audit-1", ActorID: "operator-1", Scope: "finance",
		Action: "override", OccurredAt: time.Now(),
	})
	if err == nil {
		t.Fatal("audit evidence without reason and policy version must be rejected")
	}
}
