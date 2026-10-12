package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/demand"
)

func TestAuthenticatedOrderReadbackAndCancellation(t *testing.T) {
	service := demand.NewConformanceService(nil)
	intent, err := service.CreateIntent("бессонница")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmIntent(intent.ID, []string{"sleep"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	slots, err := service.Slots("spec-lebedeva")
	if err != nil || len(slots) == 0 {
		t.Fatalf("slots=%v err=%v", slots, err)
	}
	hold, err := service.AcquireHold(intent.ID, slots[0].ID, intent.ClientIdentityID)
	if err != nil {
		t.Fatal(err)
	}
	ins, err := service.CreateCheckout(hold.ID, intent.ClientIdentityID, "SBP")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ApplyProviderEvent(demand.ProviderEvent{
		ProviderID: ins.ProviderID, ProviderEventID: "evt-order-http",
		OrderID: ins.OrderID, AmountMinor: ins.AmountMinor,
		Currency: ins.Currency, Outcome: "CAPTURED",
	})
	if err != nil {
		t.Fatal(err)
	}
	key := []byte(strings.Repeat("z", 32))
	sessions, err := newClientSessionManager(key, nil)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := sessions.issue(intent.ClientIdentityID)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := sessions.issue("foreign-client")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(Options{Demand: service, ClientSessionKey: key, ConformanceProviderEvents: true})
	call := func(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if cookie != nil {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	for _, cookie := range []*http.Cookie{nil, foreign} {
		want := http.StatusUnauthorized
		if cookie != nil {
			want = http.StatusNotFound
		}
		read := call("GET", "/v1/orders/"+ins.OrderID, "", cookie)
		if read.Code != want {
			t.Fatalf("foreign read=%d want=%d body=%s", read.Code, want, read.Body)
		}
	}
	read := call("GET", "/v1/orders/"+ins.OrderID, "", owner)
	if read.Code != 200 || !strings.Contains(read.Body.String(), "\"payment_state\":\"CAPTURED\"") {
		t.Fatalf("owner read=%d %s", read.Code, read.Body)
	}
	payload := `{"order_id":"` + ins.OrderID + `","reason_code":"CLIENT_CANCEL"}`
	for _, cookie := range []*http.Cookie{nil, foreign} {
		want := http.StatusUnauthorized
		if cookie != nil {
			want = http.StatusNotFound
		}
		res := call("POST", "/v1/cancellations", payload, cookie)
		if res.Code != want {
			t.Fatalf("foreign cancel=%d want=%d %s", res.Code, want, res.Body)
		}
	}
	if state, err := service.ReadOrderStatus(ins.OrderID, intent.ClientIdentityID); err != nil || state.BookingState != "CONFIRMED" {
		t.Fatalf("foreign cancel changed booking=%#v %v", state, err)
	}
	res := call("POST", "/v1/cancellations", payload, owner)
	if res.Code != http.StatusCreated {
		t.Fatalf("owner cancel=%d %s", res.Code, res.Body)
	}
	read = call("GET", "/v1/orders/"+ins.OrderID, "", owner)
	if read.Code != 200 || !strings.Contains(read.Body.String(), "\"booking_state\":\"CANCELLED\"") {
		t.Fatalf("readback=%d %s", read.Code, read.Body)
	}
}
