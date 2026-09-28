package runtimepostgres

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/authz"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

func TestAuthorizationEvaluatorPersistsCrossTenantDenial(t *testing.T) {
	databaseURL := os.Getenv("APGIC_AUTHZ_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("authorization integration database not configured")
	}

	store, err := Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	auditID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	input := authz.Input{
		Principal: authz.Principal{
			ID:       "org-a-owner",
			TenantID: "org-a",
			Permissions: map[string]struct{}{
				"organization.read_private": {},
			},
		},
		Resource:      authz.ResourceRef{ID: "known-private-resource-b", TenantID: "org-b"},
		Action:        "organization.read_private",
		Risk:          authz.RiskNormal,
		Now:           now,
		CorrelationID: "integration-cross-tenant-denial",
		AuditRecordID: auditID,
	}

	evaluator := authz.Evaluator{
		PolicyVersion: "authz-policy-v1",
		Appender:      store,
	}
	result, err := evaluator.Authorize(input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != authz.Deny || result.ReasonCode != "AUTH_CROSS_TENANT_DENY" {
		t.Fatalf("cross-tenant decision=%#v", result)
	}

	var actorID, action, scope, resourceRef, reason, policyVersion, correlationID string
	var stateJSON []byte
	var occurredAt time.Time
	if err := store.db.QueryRow(
		`SELECT actor_id, action, scope, resource_ref, new_state, reason,
		        policy_version, correlation_id, occurred_at
		   FROM audit_records
		  WHERE id = $1::uuid`,
		auditID,
	).Scan(
		&actorID,
		&action,
		&scope,
		&resourceRef,
		&stateJSON,
		&reason,
		&policyVersion,
		&correlationID,
		&occurredAt,
	); err != nil {
		t.Fatal(err)
	}

	var snapshot struct {
		Decision   authz.Decision `json:"decision"`
		ReasonCode string         `json:"reason_code"`
		Action     string         `json:"action"`
		Risk       authz.Risk     `json:"risk"`
	}
	if err := json.Unmarshal(stateJSON, &snapshot); err != nil {
		t.Fatal(err)
	}
	if actorID != input.Principal.ID ||
		action != "authorization.decision" ||
		scope != input.Principal.TenantID ||
		resourceRef != "tenant/org-b/resource/known-private-resource-b" ||
		reason != "AUTH_CROSS_TENANT_DENY" ||
		policyVersion != "authz-policy-v1" ||
		correlationID != input.CorrelationID ||
		!occurredAt.Equal(now) {
		t.Fatalf("persisted denial evidence mismatch: actor=%q action=%q scope=%q resource=%q reason=%q policy=%q correlation=%q occurred_at=%s",
			actorID, action, scope, resourceRef, reason, policyVersion, correlationID, occurredAt)
	}
	if snapshot.Decision != authz.Deny ||
		snapshot.ReasonCode != "AUTH_CROSS_TENANT_DENY" ||
		snapshot.Action != input.Action ||
		snapshot.Risk != authz.RiskNormal {
		t.Fatalf("persisted decision snapshot=%#v", snapshot)
	}
}
