package runtimepostgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/audit"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

func TestProductOwnershipMutationAndAuditAreAtomic(t *testing.T) {
	databaseURL := os.Getenv("APGIC_AUTHZ_HTTP_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("authorization HTTP integration database not configured")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	identityID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	productID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO identities (id) VALUES ($1::uuid)`, identityID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO products (
			id, owner_type, owner_id, commercial_owner_ref, revenue_beneficiary_ref, author_refs
		) VALUES (
			$1::uuid, 'IDENTITY', $2::uuid, 'commercial:old', 'beneficiary:test',
			ARRAY['author:test']::text[]
		)
	`, productID, identityID); err != nil {
		t.Fatal(err)
	}

	store, err := Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	invalidAuditID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.UpdateIdentityOwnedProductCommercialOwner(
		productID,
		identityID,
		"commercial:must-rollback",
		audit.Record{
			ID:            invalidAuditID,
			ActorID:       identityID,
			Action:        "product.commercial_owner.changed",
			Scope:         identityID,
			PolicyVersion: "authz-policy-v1",
			OccurredAt:    time.Now().UTC(),
		},
	)
	if err == nil {
		t.Fatal("expected invalid audit record to abort ownership mutation")
	}
	assertProductCommercialOwner(t, db, productID, "commercial:old")

	auditID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	occurredAt := time.Date(2026, 9, 29, 15, 0, 0, 0, time.UTC)
	oldOwner, err := store.UpdateIdentityOwnedProductCommercialOwner(
		productID,
		identityID,
		"commercial:new",
		audit.Record{
			ID:            auditID,
			ActorID:       identityID,
			Action:        "product.commercial_owner.changed",
			Scope:         identityID,
			Reason:        "HIGH_RISK_OWNER_CHANGE",
			PolicyVersion: "authz-policy-v1",
			OccurredAt:    occurredAt,
			CorrelationID: "audit001-atomic",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if oldOwner != "commercial:old" {
		t.Fatalf("old commercial owner=%q want commercial:old", oldOwner)
	}
	assertProductCommercialOwner(t, db, productID, "commercial:new")

	var (
		action, actorID, scope, resourceRef, reason, policyVersion, correlationID string
		oldState, newState                                                   []byte
	)
	if err := db.QueryRow(`
		SELECT action, actor_id, scope, resource_ref, reason, policy_version,
		       correlation_id, old_state, new_state
		  FROM audit_records
		 WHERE id = $1::uuid
	`, auditID).Scan(
		&action,
		&actorID,
		&scope,
		&resourceRef,
		&reason,
		&policyVersion,
		&correlationID,
		&oldState,
		&newState,
	); err != nil {
		t.Fatal(err)
	}
	if action != "product.commercial_owner.changed" ||
		actorID != identityID ||
		scope != identityID ||
		resourceRef != productID ||
		reason != "HIGH_RISK_OWNER_CHANGE" ||
		policyVersion != "authz-policy-v1" ||
		correlationID != "audit001-atomic" {
		t.Fatalf("unexpected audit metadata action=%q actor=%q scope=%q resource=%q reason=%q policy=%q correlation=%q",
			action, actorID, scope, resourceRef, reason, policyVersion, correlationID)
	}
	assertAuditCommercialOwnerState(t, oldState, "commercial:old")
	assertAuditCommercialOwnerState(t, newState, "commercial:new")

	if _, err := db.Exec(`UPDATE audit_records SET reason = 'tampered' WHERE id = $1::uuid`, auditID); err == nil {
		t.Fatal("expected append-only audit trigger to reject update")
	}
	if _, err := db.Exec(`DELETE FROM audit_records WHERE id = $1::uuid`, auditID); err == nil {
		t.Fatal("expected append-only audit trigger to reject delete")
	}
}

func assertProductCommercialOwner(t *testing.T, db *sql.DB, productID, expected string) {
	t.Helper()
	var actual string
	if err := db.QueryRow(`SELECT commercial_owner_ref FROM products WHERE id = $1::uuid`, productID).Scan(&actual); err != nil {
		t.Fatal(err)
	}
	if actual != expected {
		t.Fatalf("commercial_owner_ref=%q want %q", actual, expected)
	}
}

func assertAuditCommercialOwnerState(t *testing.T, raw []byte, expected string) {
	t.Helper()
	var state map[string]string
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	if state["commercial_owner_ref"] != expected {
		t.Fatalf("commercial_owner_ref audit state=%q want %q", state["commercial_owner_ref"], expected)
	}
}
