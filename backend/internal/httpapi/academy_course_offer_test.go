package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type academyCheckoutQuoteStub struct{ *academyStoreStub }

func (s *academyCheckoutQuoteStub) AcademyCheckoutOffer(courseID string) (academy.CourseOffer, error) {
	return academy.CourseOffer{CourseID: courseID, AccessMode: "PAID",
		PriceMinor: 249900, Currency: "RUB", Revision: 1}, nil
}

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
	registerAcademyCourseOffers(mux, &academyCheckoutQuoteStub{&academyStoreStub{}}, sessions, nil)
	url := "/v1/academy/courses/22222222-2222-4222-8222-222222222222/checkout"
	unauthorized := httptest.NewRecorder()
	mux.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, url, nil))
	if unauthorized.Code == http.StatusOK || unauthorized.Code == http.StatusCreated {
		t.Fatalf("unsigned checkout succeeded: %d", unauthorized.Code)
	}
	authorized := httptest.NewRequest(http.MethodPost, url, strings.NewReader("{}"))
	authorized.AddCookie(cookie)
	authorized.Header.Set("Idempotency-Key", "academy-order-001")
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, authorized)
	if recorder.Code != http.StatusServiceUnavailable ||
		!strings.Contains(recorder.Body.String(), "ACADEMY_EXTERNAL_CHECKOUT_NOT_CONFIGURED") ||
		!strings.Contains(recorder.Body.String(), "APGIC не принимает деньги") {
		t.Fatalf("bank-unavailable checkout response=%d %s", recorder.Code, recorder.Body.String())
	}
}

type academyCheckoutReadyStub struct {
	*academyCheckoutQuoteStub
	buyer, course, key string
	calls              int
	redirect           academyBuyerCheckoutRedirect
}

func (s *academyCheckoutReadyStub) AcademyStartBankCheckout(_ context.Context, buyer, course, key string) (academyBuyerCheckoutRedirect, error) {
	s.calls++
	s.buyer, s.course, s.key = buyer, course, key
	return s.redirect, nil
}
func TestAcademyBankBuyerCheckoutDoesNotTrustClientEconomics(t *testing.T) {
	sessions, err := newClientSessionManager([]byte(strings.Repeat("c", 32)), nil)
	if err != nil {
		t.Fatal(err)
	}
	cookie, err := sessions.issue("11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	stub := &academyCheckoutReadyStub{academyCheckoutQuoteStub: &academyCheckoutQuoteStub{&academyStoreStub{}}}
	stub.redirect = academyBuyerCheckoutRedirect{
		OrderID:    "33333333-3333-4333-8333-333333333333",
		PaymentURL: "https://securepay.tinkoff.ru/checkout", State: "SENT",
	}
	mux := http.NewServeMux()
	registerAcademyCourseOffers(mux, stub, sessions, nil)
	path := "/v1/academy/courses/22222222-2222-4222-8222-222222222222/checkout"
	send := func(key, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.AddCookie(cookie)
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		out := httptest.NewRecorder()
		mux.ServeHTTP(out, req)
		return out
	}
	if got := send("", "{}"); got.Code != http.StatusBadRequest || stub.calls != 0 {
		t.Fatalf("missing idempotency key reached PSP: %d calls=%d", got.Code, stub.calls)
	}
	attacker := send("academy-order-001", `{"amount_minor":1,"terminal":"foreign","seller_id":"other"}`)
	if attacker.Code != http.StatusBadRequest || stub.calls != 0 {
		t.Fatalf("client economics reached PSP: %d calls=%d", attacker.Code, stub.calls)
	}
	got := send("academy-order-001", "{}")
	if got.Code != http.StatusCreated || stub.calls != 1 {
		t.Fatalf("trusted redirect not returned: %d body=%s calls=%d", got.Code, got.Body.String(), stub.calls)
	}
	if stub.buyer != "11111111-1111-4111-8111-111111111111" ||
		stub.course != "22222222-2222-4222-8222-222222222222" || stub.key != "academy-order-001" {
		t.Fatalf("untrusted buyer/checkout identity forwarded: %s %s %s", stub.buyer, stub.course, stub.key)
	}
	if !strings.Contains(got.Body.String(), "\"state\":\"SENT\"") ||
		strings.Contains(got.Body.String(), "\"amount_minor\"") {
		t.Fatalf("checkout redirect unexpectedly grants rights/reflects client economics: %s", got.Body.String())
	}
	stub.redirect.PaymentURL = "https://tinkoff.ru.evil.example/checkout"
	got = send("academy-order-002", "{}")
	if got.Code != http.StatusServiceUnavailable {
		t.Fatalf("untrusted redirect accepted: %d %s", got.Code, got.Body.String())
	}
}

func TestAcademyBuyerCheckoutKeyRejectsRawDuplicateCasing(t *testing.T) {
 cases:=[]http.Header{
  {"Idempotency-Key":{"academy-order-001"},"idempotency-key":{"academy-order-002"}},
  {"Idempotency-Key":{"academy-order-001","academy-order-002"}},
  {"idempotency-key":{}},
  {"Idempotency-Key":{"abc"}},
  {"Idempotency-Key":{"../../sneaky"}},
 }
 for _,headers:=range cases{
  if key,ok:=academyBuyerCheckoutKey(headers);ok{
   t.Fatalf("accepted ambiguous/untrusted checkout key %q %#v",key,headers)
  }
 }
 if key,ok:=academyBuyerCheckoutKey(http.Header{"idempotency-key":{"academy-order-001"}});!ok||key!="academy-order-001"{
  t.Fatalf("rejected valid case-insensitive key: %q %v",key,ok)
 }
}
