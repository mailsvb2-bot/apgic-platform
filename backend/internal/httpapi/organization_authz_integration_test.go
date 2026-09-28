package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/runtimepostgres"
)

func TestCrossTenantOrganizationHTTPDeniesWithoutDisclosureAndAudits(t *testing.T) {
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
	orgA, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	orgB, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO identities (id) VALUES ($1::uuid)`, identityID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO organizations (id, name, status) VALUES
		  ($1::uuid, 'Organization A private', 'ACTIVE'),
		  ($2::uuid, 'TOP SECRET ORGANIZATION B', 'ACTIVE')
	`, orgA, orgB); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO organization_memberships (organization_id, identity_id, status)
		VALUES ($1::uuid, $2::uuid, 'ACTIVE')
	`, orgA, identityID); err != nil {
		t.Fatal(err)
	}

	store, err := runtimepostgres.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	key := []byte("authz-http-integration-key-000000000000")
	sessions, err := newClientSessionManager(key, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := sessions.issue(identityID)
	if err != nil {
		t.Fatal(err)
	}

	handler := New(Options{
		ClientSessionKey: key,
		OrganizationAuth: store,
		Now:              func() time.Time { return now },
	})

	request := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgB+"/private-profile", nil)
	request.AddCookie(cookie)
	request.Header.Set("X-Organization-Context", orgA)
	request.Header.Set("X-Correlation-Id", "auth001-db-api-negative")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("cross-tenant status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "TOP SECRET ORGANIZATION B") {
		t.Fatalf("cross-tenant response disclosed private resource: %s", recorder.Body.String())
	}

	var envelope struct {
		Code              string   `json:"code"`
		MessageSafe       string   `json:"message_safe"`
		PolicyReasonCodes []string `json:"policy_reason_codes"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != "AUTH_CROSS_TENANT_DENY" ||
		len(envelope.PolicyReasonCodes) != 1 ||
		envelope.PolicyReasonCodes[0] != "AUTH_CROSS_TENANT_DENY" {
		t.Fatalf("cross-tenant error envelope=%#v", envelope)
	}

	var actorID, scope, resourceRef, reason, correlationID string
	var stateJSON []byte
	if err := db.QueryRow(`
		SELECT actor_id, scope, resource_ref, reason, correlation_id, new_state
		  FROM audit_records
		 WHERE correlation_id = 'auth001-db-api-negative'
		 ORDER BY occurred_at DESC
		 LIMIT 1
	`).Scan(&actorID, &scope, &resourceRef, &reason, &correlationID, &stateJSON); err != nil {
		t.Fatal(err)
	}
	if actorID != identityID ||
		scope != orgA ||
		resourceRef != "tenant/"+orgB+"/resource/private-profile" ||
		reason != "AUTH_CROSS_TENANT_DENY" ||
		correlationID != "auth001-db-api-negative" {
		t.Fatalf("audit mismatch actor=%q scope=%q resource=%q reason=%q correlation=%q state=%s",
			actorID, scope, resourceRef, reason, correlationID, stateJSON)
	}

	own := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgA+"/private-profile", nil)
	own.AddCookie(cookie)
	own.Header.Set("X-Organization-Context", orgA)
	own.Header.Set("X-Correlation-Id", "auth001-positive")
	ownRecorder := httptest.NewRecorder()
	handler.ServeHTTP(ownRecorder, own)
	if ownRecorder.Code != http.StatusOK || !strings.Contains(ownRecorder.Body.String(), "Organization A private") {
		t.Fatalf("same-tenant status=%d body=%s", ownRecorder.Code, ownRecorder.Body.String())
	}

	forged := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgB+"/private-profile", nil)
	forged.AddCookie(cookie)
	forged.Header.Set("X-Organization-Context", orgB)
	forgedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(forgedRecorder, forged)
	if forgedRecorder.Code != http.StatusForbidden || strings.Contains(forgedRecorder.Body.String(), "TOP SECRET ORGANIZATION B") {
		t.Fatalf("forged tenant context status=%d body=%s", forgedRecorder.Code, forgedRecorder.Body.String())
	}
}
