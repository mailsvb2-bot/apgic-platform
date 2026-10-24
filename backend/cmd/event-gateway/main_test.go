package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGatewayRefusesUnauthenticatedDelivery(t *testing.T) {
	h := handler{principal: "outbox-service", token: strings.Repeat("x", 32)}
	for _, tc := range []struct {
		name, method, principal, scope, auth string
		expected                             int
	}{
		{"no credential", "POST", "", "", "", http.StatusUnauthorized},
		{"wrong credential", "POST", "outbox-service", "event:deliver", "***", http.StatusUnauthorized},
		{"wrong scope", "POST", "outbox-service", "event:admin", "Bearer " + strings.Repeat("x", 32), http.StatusUnauthorized},
		{"wrong principal", "POST", "foreign", "event:deliver", "Bearer " + strings.Repeat("x", 32), http.StatusUnauthorized},
		{"wrong method", "GET", "", "", "", http.StatusMethodNotAllowed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/events", strings.NewReader("{}"))
			req.Header.Set("X-APGIC-Service-Principal", tc.principal)
			req.Header.Set("X-APGIC-Service-Scope", tc.scope)
			req.Header.Set("Authorization", tc.auth)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.expected {
				t.Fatalf("got %d expected %d", rec.Code, tc.expected)
			}
		})
	}
}
func TestGatewayRejectsUnknownEventBeforeDatabase(t *testing.T) {
	token := strings.Repeat("q", 32)
	h := handler{principal: "outbox-service", token: token}
	for _, s := range []string{
		`{"event_id":"x","idempotency_key":"y","event_type":"finance.payout_executed","schema_version":"1","aggregate_version":1,"producer":"booking-ledger","payload":{}}`,
		`{"event_id":"x","idempotency_key":"y","event_type":"booking.ledger_committed","schema_version":"1","aggregate_version":1,"producer":"booking-ledger","payload":{"unexpected":"data"}}`,
		`{"event_type":"booking.ledger_committed","unknown":"value"}`,
	} {
		req := httptest.NewRequest("POST", "/events", strings.NewReader(s))
		req.Header.Set("X-APGIC-Service-Principal", "outbox-service")
		req.Header.Set("X-APGIC-Service-Scope", "event:deliver")
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code == 200 {
			t.Fatalf("accepted unsupported event %q", s)
		}
	}
}
