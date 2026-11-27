package payments

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type tbankRoundTrip func(*http.Request) (*http.Response, error)

func (f tbankRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
func tbankReply(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}
}
func validBankOrder() TBankInitRequest {
	return TBankInitRequest{
		OrderID: "abc_123", AmountMinor: 259900, Description: "Покупка курса",
		Receipt:         json.RawMessage(`{"Taxation":"usn_income","Items":[{"Name":"Курс","Price":259900,"Quantity":1,"Amount":259900,"Tax":"none","PaymentObject":"service"}]}`),
		NotificationURL: "https://apgic.ru/v1/payments/tbank/notification",
	}
}
func TestTBankInitSignedAndNoEntitlement(t *testing.T) {
	calls := 0
	client := &http.Client{Timeout: 5 * time.Second, Transport: tbankRoundTrip(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.String() != tbankInitEndpoint || req.Method != http.MethodPost {
			t.Fatalf("wrong bank endpoint")
		}
		var input map[string]any
		if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if input["OrderId"] != "abc_123" || input["Amount"] != float64(259900) {
			t.Fatalf("unexpected bank body")
		}
		if err := VerifyTBankNotificationToken(input, "test-secret"); err != nil {
			t.Fatalf("wrong signature: %v", err)
		}
		if _, ok := input["Password"]; ok {
			t.Fatal("secret leaked")
		}
		return tbankReply(200, `{"Success":true,"ErrorCode":"0","PaymentId":"7654321","OrderId":"abc_123","Status":"NEW","PaymentURL":"https://securepay.tinkoff.ru/payment/example"}`), nil
	})}
	bank, err := NewTBankAcquiring("test-terminal", "test-secret", client)
	if err != nil {
		t.Fatal(err)
	}
	result, err := bank.Init(context.Background(), validBankOrder())
	if err != nil || result.PaymentID != "7654321" || result.Status != "NEW" || calls != 1 {
		t.Fatalf("init %+v error %v calls %d", result, err, calls)
	}
}
func TestTBankInitRejectsUnsafeResponse(t *testing.T) {
	invalid := []string{
		`{"Success":true,"ErrorCode":"0","PaymentId":"1","OrderId":"abc_123","Status":"AUTHORIZED","PaymentURL":"https://securepay.tinkoff.ru/p"}`,
		`{"Success":false,"ErrorCode":"0","PaymentId":"1","OrderId":"abc_123","Status":"NEW","PaymentURL":"https://securepay.tinkoff.ru/p"}`,
		`{"Success":true,"ErrorCode":"0","PaymentId":"1","OrderId":"another","Status":"NEW","PaymentURL":"https://securepay.tinkoff.ru/p"}`,
		`{"Success":true,"ErrorCode":"0","PaymentId":"1","OrderId":"abc_123","Status":"NEW","PaymentURL":"https://tinkoff.ru.evil.example/p"}`,
		`{"Success":true,"ErrorCode":"0","PaymentId":"1","OrderId":"abc_123","Status":"NEW","PaymentURL":"http://securepay.tinkoff.ru/p"}`,
		`{"Success":true,"ErrorCode":"0","PaymentId":"1","OrderId":"abc_123","Status":"NEW","PaymentURL":"https://securepay.tinkoff.ru:444/p"}`,
		`{"Success":true,"ErrorCode":"0","PaymentId":"invalid","OrderId":"abc_123","Status":"NEW","PaymentURL":"https://securepay.tinkoff.ru/p"}`,
	}
	for _, body := range invalid {
		bank, _ := NewTBankAcquiring("terminal", "secret", &http.Client{Transport: tbankRoundTrip(func(*http.Request) (*http.Response, error) { return tbankReply(200, body), nil })})
		if _, err := bank.Init(context.Background(), validBankOrder()); err == nil {
			t.Fatalf("accepted: %s", body)
		}
	}
}
func TestTBankInitRejectsUnsafeOrderBeforeNetwork(t *testing.T) {
	calls := 0
	bank, _ := NewTBankAcquiring("terminal", "secret", &http.Client{Transport: tbankRoundTrip(func(*http.Request) (*http.Response, error) { calls++; return tbankReply(200, "{}"), nil })})
	for _, change := range []func(*TBankInitRequest){
		func(o *TBankInitRequest) { o.OrderID = "../other" },
		func(o *TBankInitRequest) { o.AmountMinor = 0 },
		func(o *TBankInitRequest) { o.NotificationURL = "https://evil.example/webhook" },
		func(o *TBankInitRequest) { o.NotificationURL = "http://apgic.ru/v1/payments/tbank/notification" },
		func(o *TBankInitRequest) { o.Receipt = nil },
		func(o *TBankInitRequest) { o.Description = "invalid\r\nheader" },
	} {
		v := validBankOrder()
		change(&v)
		if _, err := bank.Init(context.Background(), v); err == nil {
			t.Fatal("unsafe order accepted")
		}
	}
	if calls != 0 {
		t.Fatalf("made %d unsafe bank calls", calls)
	}
}
