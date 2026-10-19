package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/audit"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/finance"
)

type testFinanceOpsStore struct {
	factorActive bool
	operator     string
	audits       []audit.Record
	resolved     []finance.DiscrepancyResolutionRequest
}

func (s *testFinanceOpsStore) Append(record audit.Record) error {
	s.audits = append(s.audits, record)
	return nil
}

func (s *testFinanceOpsStore) ActiveFinanceOperatorGrant(_ context.Context, identity string, _ time.Time) (string, bool, error) {
	if identity == s.operator {
		return "00000000-0000-4000-8000-0000000000f1", true, nil
	}
	return "", false, nil
}

func (s *testFinanceOpsStore) FinanceMFAActiveAt(_ context.Context, _ string, _ time.Time) (bool, error) {
	return s.factorActive, nil
}

func (s *testFinanceOpsStore) ListOpenFinanceDiscrepancies(context.Context, int) ([]finance.OpenDiscrepancy, error) {
	return []finance.OpenDiscrepancy{{DiscrepancyID: "00000000-0000-4000-8000-0000000000d1", Kind: "PROVIDER_LEDGER_MISMATCH", Currency: "RUB"}}, nil
}

func (s *testFinanceOpsStore) ResolveFinanceDiscrepancy(_ context.Context, req finance.DiscrepancyResolutionRequest) (finance.DiscrepancyResolution, error) {
	if len(s.resolved) > 0 {
		return finance.DiscrepancyResolution{}, finance.ErrDiscrepancyAlreadyResolved
	}
	s.resolved = append(s.resolved, req)
	return finance.DiscrepancyResolution{ResolutionID: "r1", DiscrepancyID: req.DiscrepancyID, Kind: req.Kind, ResolvedBy: req.OperatorID, ResolvedAt: req.ResolvedAt}, nil
}

func TestFinanceDiscrepancyResolutionRequiresGrantStepUpAndAudit(t *testing.T) {
	now := time.Date(2026, 10, 15, 1, 0, 0, 0, time.UTC)
	key := []byte(strings.Repeat("o", 32))
	operator, stranger := "00000000-0000-4000-8000-0000000000b1", "00000000-0000-4000-8000-0000000000b2"
	store := &testFinanceOpsStore{operator: operator}
	handler := New(Options{ClientSessionKey: key, FinanceOperations: store, Now: func() time.Time { return now }})
	sessions, err := newClientSessionManager(key, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	stepUps, err := newStepUpManager(key, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	cookieFor := func(identity string) (*http.Cookie, string) {
		cookie, err := sessions.issue(identity)
		if err != nil {
			t.Fatal(err)
		}
		probe := httptest.NewRequest(http.MethodGet, "/", nil)
		probe.AddCookie(cookie)
		_, ref, err := sessions.identityAndReferenceFromRequest(probe)
		if err != nil {
			t.Fatal(err)
		}
		return cookie, ref
	}
	do := func(method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("content-type", "application/json")
		for _, c := range cookies {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	const list = "/v1/finance/operations/discrepancies"
	const resolve = "/v1/finance/operations/discrepancies/00000000-0000-4000-8000-0000000000d1/resolutions"
	body := `{"idempotency_key":"k1","kind":"OPERATIONAL_EXCEPTION_ACCEPTED","reason":"accepted by finance ops","evidence_refs":["ticket/1"]}`

	if rec := do(http.MethodGet, list, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous queue read: %d", rec.Code)
	}
	strangerCookie, strangerRef := cookieFor(stranger)
	if rec := do(http.MethodGet, list, "", strangerCookie); rec.Code != http.StatusForbidden {
		t.Fatalf("ungranted identity read queue: %d", rec.Code)
	}
	strangerStepUp, _ := stepUps.issue(stranger, strangerRef, "WEBAUTHN", now)
	if rec := do(http.MethodPost, resolve, body, strangerCookie, strangerStepUp); rec.Code != http.StatusForbidden || len(store.resolved) != 0 {
		t.Fatalf("ungranted identity resolved discrepancy: %d", rec.Code)
	}

	operatorCookie, operatorRef := cookieFor(operator)
	if rec := do(http.MethodGet, list, "", operatorCookie); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "PROVIDER_LEDGER_MISMATCH") {
		t.Fatalf("operator queue read: %d %s", rec.Code, rec.Body.String())
	}
	for _, invalidLimit := range []string{"?limit=", "?limit=0", "?limit=-1", "?limit=201", "?limit=abc", "?limit=1&limit=2", "?limit=999999999999999999999"} {
		if rec := do(http.MethodGet, list+invalidLimit, "", operatorCookie); rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid finance queue limit %q accepted: %d %s", invalidLimit, rec.Code, rec.Body.String())
		}
	}
	for _, validLimit := range []string{"?limit=1", "?limit=200"} {
		if rec := do(http.MethodGet, list+validLimit, "", operatorCookie); rec.Code != http.StatusOK {
			t.Fatalf("valid finance queue limit %q rejected: %d %s", validLimit, rec.Code, rec.Body.String())
		}
	}
	if rec := do(http.MethodPost, resolve, body, operatorCookie); rec.Code != http.StatusPreconditionRequired || len(store.resolved) != 0 {
		t.Fatalf("resolution without step-up: %d", rec.Code)
	}
	staleStepUp, _ := stepUps.issue(operator, operatorRef, "WEBAUTHN", now.Add(-time.Hour))
	if rec := do(http.MethodPost, resolve, body, operatorCookie, staleStepUp); rec.Code != http.StatusPreconditionRequired {
		t.Fatalf("stale step-up accepted: %d", rec.Code)
	}
	freshStepUp, _ := stepUps.issue(operator, operatorRef, "WEBAUTHN", now)
	// High-risk finance resolution must be an unambiguous single JSON object.
	// A valid prefix followed by another object or JSON token must not produce
	// any decision, while JSON whitespace after the object is permitted.
	for _, invalid := range []string{
		body + body,
		body + `{"kind":"PROVIDER_CORRECTION_CONFIRMED"}`,
		body + `null`,
		body + `[]`,
		body + `garbage`,
		body + strings.Repeat(" ", financeOpsMaxRequestBytes),
	} {
		if rec := do(http.MethodPost, resolve, invalid, operatorCookie, freshStepUp); rec.Code != http.StatusBadRequest || len(store.resolved) != 0 {
			t.Fatalf("ambiguous finance resolution accepted: status=%d body=%q", rec.Code, rec.Body.String())
		}
	}
	forged := `{"idempotency_key":"k1","kind":"OPERATIONAL_EXCEPTION_ACCEPTED","reason":"accepted by finance ops","evidence_refs":["ticket/1"],"operator_id":"` + stranger + `"}`
	if rec := do(http.MethodPost, resolve, forged, operatorCookie, freshStepUp); rec.Code != http.StatusBadRequest {
		t.Fatalf("body-supplied operator accepted: %d", rec.Code)
	}
	rec := do(http.MethodPost, resolve, body, operatorCookie, freshStepUp)
	if rec.Code != http.StatusCreated || len(store.resolved) != 1 {
		t.Fatalf("granted resolution: %d %s", rec.Code, rec.Body.String())
	}
	got := store.resolved[0]
	if got.OperatorID != operator || got.StepUpSessionRef != operatorRef || !strings.HasPrefix(got.AuditRef, "audit/") || !got.ResolvedAt.Equal(now) {
		t.Fatalf("resolution not bound to trusted operator context: %+v", got)
	}
	if rec := do(http.MethodPost, resolve, body, operatorCookie, freshStepUp); rec.Code != http.StatusConflict {
		t.Fatalf("second resolution: %d", rec.Code)
	}
	if len(store.audits) < 6 {
		t.Fatalf("authorization decisions not audited: %d", len(store.audits))
	}
}

func TestFinanceTOTPRevokeInvalidatesActiveCookieBeforeResolution(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	operator := "00000000-0000-4000-8000-0000000000b1"
	key := []byte(strings.Repeat("m", 32))
	store := &testFinanceOpsStore{operator: operator, factorActive: false}
	handler := New(Options{ClientSessionKey: key, FinanceOperations: store, Now: func() time.Time { return now }})
	sessions, err := newClientSessionManager(key, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	manager, err := newStepUpManager(key, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	session, err := sessions.issue(operator)
	if err != nil {
		t.Fatal(err)
	}
	probe := httptest.NewRequest(http.MethodGet, "/", nil)
	probe.AddCookie(session)
	_, ref, err := sessions.identityAndReferenceFromRequest(probe)
	if err != nil {
		t.Fatal(err)
	}
	verifiedCookie, err := manager.issue(operator, ref, "TOTP", now)
	if err != nil {
		t.Fatal(err)
	}
	const route = "/v1/finance/operations/discrepancies/00000000-0000-4000-8000-0000000000d1/resolutions"
	const body = `{"idempotency_key":"step-up-revoke","kind":"OPERATIONAL_EXCEPTION_ACCEPTED","reason":"approved by finance operations","evidence_refs":["ticket/1"]}`
	resolve := func() int {
		request := httptest.NewRequest(http.MethodPost, route, strings.NewReader(body))
		request.AddCookie(session)
		request.AddCookie(verifiedCookie)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder.Code
	}
	if status := resolve(); status != http.StatusPreconditionRequired || len(store.resolved) != 0 {
		t.Fatalf("revoked TOTP factor still authorized a financial decision: %d", status)
	}
	store.factorActive = true
	if status := resolve(); status != http.StatusCreated || len(store.resolved) != 1 {
		t.Fatalf("currently active TOTP was not accepted: %d", status)
	}
}
