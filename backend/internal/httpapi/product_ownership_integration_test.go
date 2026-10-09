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

func TestHighRiskProductOwnershipRequiresFreshStepUpAndAudits(t *testing.T) {
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

	store, err := runtimepostgres.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Date(2026, 9, 29, 5, 0, 0, 0, time.UTC)
	key := []byte("auth002-step-up-integration-key-0000000000")
	sessions, err := newClientSessionManager(key, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	sessionCookie, err := sessions.issue(identityID)
	if err != nil {
		t.Fatal(err)
	}
	stepUps, err := newStepUpManager(key, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	sessionProbe := httptest.NewRequest(http.MethodGet, "/", nil)
	sessionProbe.AddCookie(sessionCookie)
	_, sessionRef, err := sessions.identityAndReferenceFromRequest(sessionProbe)
	if err != nil {
		t.Fatal(err)
	}

	handler := New(Options{
		ClientSessionKey: key,
		ProductOwnership: store,
		Now:              func() time.Time { return now },
	})

	request := func(correlation, owner string, stepUpCookie *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPatch, "/v1/products/"+productID+"/commercial-owner",
			strings.NewReader(`{"commercial_owner_ref":"`+owner+`"}`))
		req.Header.Set("content-type", "application/json")
		req.Header.Set("X-Correlation-Id", correlation)
		req.AddCookie(sessionCookie)
		if stepUpCookie != nil {
			req.AddCookie(stepUpCookie)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	missing := request("auth002-missing", "commercial:missing", nil)
	if missing.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing step-up status=%d body=%s", missing.Code, missing.Body.String())
	}
	assertStepUpEnvelope(t, missing, "AUTH_STEP_UP_REQUIRED")
	assertCommercialOwner(t, db, productID, "commercial:old")
	assertAuditReason(t, db, "auth002-missing", "AUTH_STEP_UP_REQUIRED")

	staleCookie, err := stepUps.issue(identityID, sessionRef, "WEBAUTHN", now.Add(-11*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	stale := request("auth002-stale", "commercial:stale", staleCookie)
	if stale.Code != http.StatusPreconditionRequired {
		t.Fatalf("stale step-up status=%d body=%s", stale.Code, stale.Body.String())
	}
	assertStepUpEnvelope(t, stale, "AUTH_STEP_UP_REQUIRED")
	assertCommercialOwner(t, db, productID, "commercial:old")
	assertAuditReason(t, db, "auth002-stale", "AUTH_STEP_UP_REQUIRED")

	foreignProductID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	foreign := httptest.NewRequest(http.MethodPatch, "/v1/products/"+foreignProductID+"/commercial-owner",
		strings.NewReader(`{"commercial_owner_ref":"commercial:foreign"}`))
	foreign.Header.Set("content-type", "application/json")
	foreign.Header.Set("X-Correlation-Id", "auth002-foreign")
	foreign.AddCookie(sessionCookie)
	foreignRecorder := httptest.NewRecorder()
	handler.ServeHTTP(foreignRecorder, foreign)
	if foreignRecorder.Code != http.StatusNotFound {
		t.Fatalf("foreign product status=%d body=%s", foreignRecorder.Code, foreignRecorder.Body.String())
	}
	assertAuditReason(t, db, "auth002-foreign", "AUTH_PERMISSION_DENIED")

	wrongSessionCookie, err := stepUps.issue(identityID, "session:other", "WEBAUTHN", now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	wrongSession := request("auth002-wrong-session", "commercial:wrong-session", wrongSessionCookie)
	if wrongSession.Code != http.StatusPreconditionRequired {
		t.Fatalf("wrong-session step-up status=%d body=%s", wrongSession.Code, wrongSession.Body.String())
	}
	assertStepUpEnvelope(t, wrongSession, "AUTH_STEP_UP_REQUIRED")
	assertCommercialOwner(t, db, productID, "commercial:old")
	assertAuditReason(t, db, "auth002-wrong-session", "AUTH_STEP_UP_REQUIRED")

	freshCookie, err := stepUps.issue(identityID, sessionRef, "WEBAUTHN", now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	fresh := request("auth002-fresh", "commercial:new", freshCookie)
	if fresh.Code != http.StatusOK {
		t.Fatalf("fresh step-up status=%d body=%s", fresh.Code, fresh.Body.String())
	}
	assertCommercialOwner(t, db, productID, "commercial:new")
	assertAuditReason(t, db, "auth002-fresh", "AUTH_ALLOWED")
	assertHighRiskSecurityEvidence(t, db, "auth002-fresh", identityID, sessionRef, "WEBAUTHN")
}

func assertStepUpEnvelope(t *testing.T, recorder *httptest.ResponseRecorder, expected string) {
	t.Helper()
	var envelope struct {
		Code              string   `json:"code"`
		PolicyReasonCodes []string `json:"policy_reason_codes"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != expected ||
		len(envelope.PolicyReasonCodes) != 1 ||
		envelope.PolicyReasonCodes[0] != expected {
		t.Fatalf("step-up envelope=%#v", envelope)
	}
}

func assertCommercialOwner(t *testing.T, db *sql.DB, productID, expected string) {
	t.Helper()
	var actual string
	if err := db.QueryRow(`SELECT commercial_owner_ref FROM products WHERE id = $1::uuid`, productID).Scan(&actual); err != nil {
		t.Fatal(err)
	}
	if actual != expected {
		t.Fatalf("commercial_owner_ref=%q want %q", actual, expected)
	}
}

func assertAuditReason(t *testing.T, db *sql.DB, correlationID, expected string) {
	t.Helper()
	var reason string
	if err := db.QueryRow(`
		SELECT reason
		  FROM audit_records
		 WHERE correlation_id = $1
		 ORDER BY occurred_at DESC
		 LIMIT 1
	`, correlationID).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if reason != expected {
		t.Fatalf("audit reason=%q want %q", reason, expected)
	}
}

func assertHighRiskSecurityEvidence(t *testing.T, db *sql.DB, correlationID, principalID, sessionRef, method string) {
	t.Helper()
	var raw []byte
	if err := db.QueryRow(`
		SELECT new_state
		  FROM audit_records
		 WHERE correlation_id = $1
		   AND action = 'authorization.decision'
		 ORDER BY occurred_at DESC
		 LIMIT 1
	`, correlationID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		SecurityEvidence struct {
			PrincipalID   string     `json:"principal_id"`
			SessionRef    string     `json:"session_ref"`
			Method        string     `json:"method"`
			StepUpAt      *time.Time `json:"step_up_at"`
			PolicyVersion string     `json:"policy_version"`
			Decision      string     `json:"decision"`
		} `json:"security_evidence"`
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	evidence := snapshot.SecurityEvidence
	if evidence.PrincipalID != principalID ||
		evidence.SessionRef != sessionRef ||
		evidence.Method != method ||
		evidence.StepUpAt == nil ||
		evidence.PolicyVersion != "authz-policy-v1" ||
		evidence.Decision != "ALLOW" {
		t.Fatalf("security evidence=%#v", evidence)
	}
}
