package payments

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestTBankGetStateConfirmsPersistedOrderOnly(t *testing.T) {
	expect := TBankCaptureExpectation{TerminalKey: "merchant", OrderID: "academy123", PaymentID: "234567", AmountMinor: 49900}
	calls := 0
	bank, err := NewTBankAcquiring("merchant", "bank-secret", &http.Client{Transport: tbankRoundTrip(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.String() != tbankGetStateEndpoint || req.Method != http.MethodPost {
			t.Fatal("wrong bank route")
		}
		var fields map[string]any
		if err := json.NewDecoder(req.Body).Decode(&fields); err != nil {
			t.Fatal(err)
		}
		if fields["PaymentId"] != "234567" || fields["TerminalKey"] != "merchant" {
			t.Fatal("wrong persisted payment identity")
		}
		if err := VerifyTBankNotificationToken(fields, "bank-secret"); err != nil {
			t.Fatalf("unsigned GetState request: %v", err)
		}
		if fields["Password"] != nil {
			t.Fatal("bank password leaked")
		}
		return tbankReply(200, `{"TerminalKey":"merchant","OrderId":"academy123","PaymentId":234567,"Amount":49900,"Status":"CONFIRMED","Success":true,"ErrorCode":"0"}`), nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	captured, err := bank.GetState(context.Background(), expect)
	if err != nil || captured.AmountMinor != expect.AmountMinor || calls != 1 {
		t.Fatalf("status %+v err %v calls %d", captured, err, calls)
	}
}
func TestTBankGetStateRejectsMismatchedOrUnsettledStatus(t *testing.T) {
	expect := TBankCaptureExpectation{TerminalKey: "merchant", OrderID: "academy123", PaymentID: "234567", AmountMinor: 49900}
	bad := []string{
		`{"TerminalKey":"merchant","OrderId":"academy123","PaymentId":234567,"Amount":49900,"Amount":49901,"Status":"CONFIRMED","Success":true,"ErrorCode":"0"}`,
		`{"TerminalKey":"merchant","OrderId":"academy123","OrderId":"different","PaymentId":234567,"Amount":49900,"Status":"CONFIRMED","Success":true,"ErrorCode":"0"}`,
		`{"TerminalKey":"merchant","OrderId":"academy123","PaymentId":234567,"Amount":49900,"Status":"AUTHORIZED","Success":true,"ErrorCode":"0"}`,
		`{"TerminalKey":"merchant","OrderId":"academy123","PaymentId":234567,"Amount":49900,"Status":"REFUNDED","Success":true,"ErrorCode":"0"}`,
		`{"TerminalKey":"merchant","OrderId":"academy123","PaymentId":234567,"Amount":49901,"Status":"CONFIRMED","Success":true,"ErrorCode":"0"}`,
		`{"TerminalKey":"merchant","OrderId":"another","PaymentId":234567,"Amount":49900,"Status":"CONFIRMED","Success":true,"ErrorCode":"0"}`,
		`{"TerminalKey":"foreign","OrderId":"academy123","PaymentId":234567,"Amount":49900,"Status":"CONFIRMED","Success":true,"ErrorCode":"0"}`,
		`{"TerminalKey":"merchant","OrderId":"academy123","PaymentId":234568,"Amount":49900,"Status":"CONFIRMED","Success":true,"ErrorCode":"0"}`,
		`{"TerminalKey":"merchant","OrderId":"academy123","PaymentId":234567,"Amount":49900,"Status":"CONFIRMED","Success":false,"ErrorCode":"0"}`,
		`{"TerminalKey":"merchant","OrderId":"academy123","PaymentId":234567,"Amount":49900,"Status":"CONFIRMED","Success":true,"ErrorCode":"51"}`,
	}
	for _, body := range bad {
		bank, _ := NewTBankAcquiring("merchant", "bank-secret", &http.Client{Transport: tbankRoundTrip(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})})
		if _, err := bank.GetState(context.Background(), expect); err == nil {
			t.Fatalf("accepted unsafe GetState %s", body)
		}
	}
}
func TestTBankGetStateRequiresStoredTerminal(t *testing.T) {
	bank, _ := NewTBankAcquiring("merchant", "bank-secret", &http.Client{Transport: tbankRoundTrip(func(*http.Request) (*http.Response, error) { t.Fatal("must not call bank"); return nil, nil })})
	expect := TBankCaptureExpectation{TerminalKey: "foreign", OrderID: "academy123", PaymentID: "234567", AmountMinor: 49900}
	if _, err := bank.GetState(context.Background(), expect); err == nil {
		t.Fatal("accepted foreign terminal")
	}
	expect.TerminalKey = "merchant"
	expect.PaymentID = "not-a-number"
	if _, err := bank.GetState(context.Background(), expect); err == nil {
		t.Fatal("accepted invalid bank payment id")
	}
}

func TestTBankCaptureAdmissionRequiresBothSignedWebhookAndGetState(t *testing.T) {
	expected := TBankCaptureExpectation{
		TerminalKey: "merchant", OrderID: "academy123", PaymentID: "234567", AmountMinor: 49900,
	}
	signed := signedTBankCapture(t, map[string]any{
		"TerminalKey": "merchant", "OrderId": "academy123",
		"PaymentId": json.Number("234567"), "Amount": json.Number("49900"),
		"Status": "CONFIRMED", "Success": true, "ErrorCode": "0",
	})
	calls := 0
	state := `{"TerminalKey":"merchant","OrderId":"academy123","PaymentId":234567,"Amount":49900,"Status":"CONFIRMED","Success":true,"ErrorCode":"0"}`
	bank, err := NewTBankAcquiring("merchant", "test-password", &http.Client{Transport: tbankRoundTrip(func(*http.Request) (*http.Response, error) {
		calls++
		return tbankReply(http.StatusOK, state), nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = bank.ConfirmNotificationWithGetState(context.Background(), signed, expected); err != nil || calls != 1 {
		t.Fatalf("valid independent evidence rejected: %v calls=%d", err, calls)
	}
	state = `{"TerminalKey":"merchant","OrderId":"academy123","PaymentId":234567,"Amount":49900,"Status":"AUTHORIZED","Success":true,"ErrorCode":"0"}`
	if _, err = bank.ConfirmNotificationWithGetState(context.Background(), signed, expected); err == nil {
		t.Fatal("accepted webhook with unconfirmed bank state")
	}
	callsBefore := calls
	if _, err = bank.ConfirmNotificationWithGetState(context.Background(), []byte(`{}`), expected); err == nil || calls != callsBefore {
		t.Fatalf("invalid notification was sent to GetState: %v", err)
	}
}
