package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/payments"
)

type academyBankCommitSpy struct{ calls int }

func (s *academyBankCommitSpy) Commit(context.Context, string, []byte, *payments.TBankAcquiring) error {
	s.calls++
	return nil
}
func TestAcademyTBankNotificationFailsClosedWithoutExplicitRelease(t *testing.T) {
	spy := &academyBankCommitSpy{}
	t.Setenv("APGIC_ACADEMY_TBANK_WEBHOOK_ENABLED", "false")
	t.Setenv("TBANK_TERMINAL_KEY", "test-terminal")
	t.Setenv("TBANK_TERMINAL_PASSWORD", "test-secret")
	mux := http.NewServeMux()
	registerAcademyBankWebhook(mux, spy)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/payments/tbank/notification",
		strings.NewReader(`{"OrderId":"11111111-1111-4111-8111-111111111111"}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusServiceUnavailable || spy.calls != 0 ||
		!strings.Contains(recorder.Body.String(), "ACADEMY_BANK_WEBHOOK_DISABLED") {
		t.Fatalf("bank release gate bypassed status=%d body=%s calls=%d", recorder.Code, recorder.Body.String(), spy.calls)
	}
}
func TestAcademyTBankNotificationRequiresTrustedBankConfiguration(t *testing.T) {
	spy := &academyBankCommitSpy{}
	t.Setenv("APGIC_ACADEMY_TBANK_WEBHOOK_ENABLED", "true")
	t.Setenv("TBANK_TERMINAL_KEY", "")
	t.Setenv("TBANK_TERMINAL_PASSWORD", "")
	t.Setenv("APGIC_TBANK_CA_BUNDLE_PATH", "")
	mux := http.NewServeMux()
	registerAcademyBankWebhook(mux, spy)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/payments/tbank/notification",
		strings.NewReader(`{"OrderId":"11111111-1111-4111-8111-111111111111"}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusServiceUnavailable || spy.calls != 0 ||
		!strings.Contains(recorder.Body.String(), "ACADEMY_BANK_CONFIG_UNAVAILABLE") {
		t.Fatalf("unconfigured bank reached capture: status=%d calls=%d body=%s", recorder.Code, spy.calls, recorder.Body.String())
	}
}
func TestAcademyTBankWebhookAbsentCommerceWriterDenied(t *testing.T) {
	t.Setenv("APGIC_ACADEMY_TBANK_WEBHOOK_ENABLED", "true")
	mux := http.NewServeMux()
	registerAcademyBankWebhook(mux, nil)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/payments/tbank/notification", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("nil canonical writer accepted bank webhook: %d", recorder.Code)
	}
}
