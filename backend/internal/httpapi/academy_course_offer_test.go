package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAcademyExternalCheckoutCannotClaimPaymentWithoutBank(t *testing.T) {
	key := []byte(strings.Repeat("c", 32))
	sessions, err := newClientSessionManager(key, nil)
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := sessions.issue("11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerAcademyCourseOffers(mux, nil, sessions, nil)
	url := "/v1/academy/courses/22222222-2222-4222-8222-222222222222/checkout"
	unauthorized := httptest.NewRecorder()
	mux.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, url, nil))
	if unauthorized.Code == http.StatusOK || unauthorized.Code == http.StatusCreated {
		t.Fatalf("unsigned checkout succeeded: %d", unauthorized.Code)
	}
	authorized := httptest.NewRequest(http.MethodPost, url, strings.NewReader("{}"))
	authorized.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, authorized)
	if recorder.Code != http.StatusServiceUnavailable ||
		!strings.Contains(recorder.Body.String(), "ACADEMY_EXTERNAL_CHECKOUT_NOT_CONFIGURED") ||
		!strings.Contains(recorder.Body.String(), "APGIC не принимает деньги") {
		t.Fatalf("bank-unavailable checkout response=%d %s", recorder.Code, recorder.Body.String())
	}
}
