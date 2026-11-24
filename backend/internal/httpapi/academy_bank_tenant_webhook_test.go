package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/payments"
)

const academyWebhookTestOrder = "11111111-1111-4111-8111-111111111111"

type academyTBankSellerResolverSpy struct {
	terminal string
	calls    int
}

func (s *academyTBankSellerResolverSpy) AcademyBankForTerminal(_ context.Context, terminal string) (*payments.TBankAcquiring, error) {
	s.calls++
	if terminal != s.terminal {
		return nil, academy.ErrDenied
	}
	return payments.NewTBankAcquiring(s.terminal, "ci-merchant-secret", nil)
}

type academyTBankSignedCommitSpy struct{ calls int }

func (s *academyTBankSignedCommitSpy) Commit(_ context.Context, order string, raw []byte, bank *payments.TBankAcquiring) error {
	s.calls++
	if order != academyWebhookTestOrder {
		return academy.ErrDenied
	}
	_, err := bank.VerifyCapturedNotification(raw, payments.TBankCaptureExpectation{
		TerminalKey: "merchant-a", OrderID: order, PaymentID: "12345", AmountMinor: 49900,
	})
	return err
}
func academySignedSellerBankWebhook(t *testing.T, terminal string) []byte {
	t.Helper()
	m := map[string]any{
		"TerminalKey": terminal, "OrderId": academyWebhookTestOrder,
		"PaymentId": json.Number("12345"), "Amount": json.Number("49900"),
		"Status": "CONFIRMED", "Success": true, "ErrorCode": "0",
	}
	signature, err := payments.TBankToken(m, "ci-merchant-secret")
	if err != nil {
		t.Fatal(err)
	}
	m["Token"] = signature
	body, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
func TestAcademyMultiSellerBankWebhookUsesApprovedTerminalAndSignedCapture(t *testing.T) {
	t.Setenv("APGIC_ACADEMY_TBANK_WEBHOOK_ENABLED", "true")
	t.Setenv("TBANK_TERMINAL_KEY", "")
	t.Setenv("TBANK_TERMINAL_PASSWORD", "")
	resolver := &academyTBankSellerResolverSpy{terminal: "merchant-a"}
	commit := &academyTBankSignedCommitSpy{}
	mux := http.NewServeMux()
	registerAcademyBankWebhookWithResolver(mux, commit, resolver)
	send := func(body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/payments/tbank/notification", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		mux.ServeHTTP(out, req)
		return out
	}
	valid := academySignedSellerBankWebhook(t, "merchant-a")
	if res := send(valid); res.Code != http.StatusOK || res.Body.String() != "OK" || commit.calls != 1 {
		t.Fatalf("signed tenant webhook not routed: status=%d body=%s calls=%d", res.Code, res.Body.String(), commit.calls)
	}
	if res := send(academySignedSellerBankWebhook(t, "merchant-b")); res.Code != http.StatusServiceUnavailable || commit.calls != 1 {
		t.Fatalf("foreign seller reached capture: %d calls=%d", res.Code, commit.calls)
	}
	forged := strings.Replace(string(valid), "\"Amount\":49900", "\"Amount\":1", 1)
	if res := send([]byte(forged)); res.Code == http.StatusOK {
		t.Fatal("changed merchant economics accepted despite signature")
	}
	if resolver.calls != 3 {
		t.Fatalf("terminal resolver not used consistently, calls=%d", resolver.calls)
	}
}
