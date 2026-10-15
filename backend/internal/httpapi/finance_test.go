package httpapi

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/connector"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/finance"
)

type testFinanceStore struct {
	applied     int
	readByOwner string
}

func (f *testFinanceStore) ApplyFinanceProviderWebhook(_ context.Context, _ connector.WebhookEnvelope, _ connector.WebhookPublicKeyResolver) (finance.EvidenceDecision, error) {
	f.applied++
	return finance.EvidenceDecision{EvidenceID: "e", Accepted: true, ReasonCode: "FINANCE_EVIDENCE_ACCEPTED"}, nil
}

func (f *testFinanceStore) ReadFinanceOrderStatus(_ context.Context, orderID, identity string) (*finance.OrderStatus, error) {
	if identity != f.readByOwner {
		return nil, nil
	}
	return &finance.OrderStatus{OrderID: orderID, Currency: "RUB", TotalAmountMinor: 20000,
		SettlementStatus: "PROVIDER_SETTLED", PayoutStatus: "PROVIDER_PAID_OUT",
		ReconciliationOutcome: "MATCHED", RevenueAssurance: "PASS", ExecutionOwner: finance.ExternalExecutionOwner}, nil
}

func TestFinanceWebhookRequiresSignatureAndStatusIsOwnerScoped(t *testing.T) {
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	keys := providerWebhookKeyMap{"provider-b/key-1": public}
	sessionKey := []byte(strings.Repeat("f", 32))
	owner, stranger := "00000000-0000-4000-8000-0000000000a1", "00000000-0000-4000-8000-0000000000a2"
	store := &testFinanceStore{readByOwner: owner}
	handler := New(Options{Finance: store, ProviderWebhookKeys: keys, ClientSessionKey: sessionKey})

	event := connector.WebhookEnvelope{
		ConnectorInstanceID: "provider-b", ExternalEventID: "settled-1", StreamID: "finance/settlement-1",
		Sequence: 1, KeyID: "key-1", OccurredAt: time.Now().UTC(),
		Payload: json.RawMessage(`{"evidence_kind":"SETTLEMENT_EXECUTED","settlement_instruction_id":"00000000-0000-4000-8000-000000000001","provider_reference":"p","gross_amount_minor":20000,"provider_fee_minor":300,"currency":"RUB"}`),
	}
	post := func(e connector.WebhookEnvelope) *httptest.ResponseRecorder {
		body, _ := json.Marshal(e)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/finance-provider-webhooks", strings.NewReader(string(body))))
		return rec
	}
	if rec := post(event); rec.Code != http.StatusUnauthorized || store.applied != 0 {
		t.Fatalf("unsigned finance evidence accepted: %d applied=%d", rec.Code, store.applied)
	}
	signed := signProviderWebhook(t, event, private)
	tampered := signed
	tampered.Payload = json.RawMessage(strings.Replace(string(signed.Payload), "300", "0", 1))
	if rec := post(tampered); rec.Code != http.StatusUnauthorized || store.applied != 0 {
		t.Fatalf("tampered finance evidence accepted: %d", rec.Code)
	}
	if rec := post(signed); rec.Code != http.StatusOK || store.applied != 1 {
		t.Fatalf("signed finance evidence rejected: %d %s", rec.Code, rec.Body.String())
	}

	sessions, err := newClientSessionManager(sessionKey, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	read := func(identity string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "/v1/finance/orders/00000000-0000-4000-8000-000000000450/status", nil)
		if identity != "" {
			cookie, err := sessions.issue(identity)
			if err != nil {
				t.Fatal(err)
			}
			request.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, request)
		return rec
	}
	if rec := read(""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous finance status read: %d", rec.Code)
	}
	if rec := read(stranger); rec.Code != http.StatusNotFound {
		t.Fatalf("foreign finance status read: %d", rec.Code)
	}
	rec := read(owner)
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" ||
		!strings.Contains(rec.Body.String(), `"execution_owner":"EXTERNAL_PROVIDER"`) ||
		strings.Contains(strings.ToLower(rec.Body.String()), "balance") {
		t.Fatalf("owner finance status: %d %s", rec.Code, rec.Body.String())
	}
}
