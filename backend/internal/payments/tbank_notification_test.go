package payments

import (
	"encoding/json"
	"testing"
)

func signedTBankCapture(t *testing.T, fields map[string]any) []byte {
	t.Helper()
	token, err := TBankToken(fields, "test-password")
	if err != nil {
		t.Fatal(err)
	}
	fields["Token"] = token
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func TestTBankCapturedNotificationBindsExactPaymentAndAmount(t *testing.T) {
	expected := TBankCaptureExpectation{TerminalKey: "test-terminal", OrderID: "abc_123", PaymentID: "12345", AmountMinor: 259900}
	original := map[string]any{"TerminalKey": "test-terminal", "OrderId": "abc_123", "PaymentId": json.Number("12345"), "Amount": json.Number("259900"), "Status": "CONFIRMED", "Success": true, "ErrorCode": "0"}
	good := signedTBankCapture(t, original)
	event, err := VerifyTBankCapturedEvent(good, "test-password", expected)
	if err != nil || event.AmountMinor != expected.AmountMinor {
		t.Fatalf("capture %+v %v", event, err)
	}
	failures := []struct {
		name   string
		change func(*TBankCaptureExpectation, map[string]any)
	}{
		{"unconfirmed", func(_ *TBankCaptureExpectation, m map[string]any) { m["Status"] = "AUTHORIZED" }},
		{"refund", func(_ *TBankCaptureExpectation, m map[string]any) { m["Status"] = "REFUNDED" }},
		{"failed", func(_ *TBankCaptureExpectation, m map[string]any) { m["Success"] = false }},
		{"changed amount", func(_ *TBankCaptureExpectation, m map[string]any) { m["Amount"] = json.Number("259901") }},
		{"foreign order", func(_ *TBankCaptureExpectation, m map[string]any) { m["OrderId"] = "foreign" }},
		{"foreign terminal", func(_ *TBankCaptureExpectation, m map[string]any) { m["TerminalKey"] = "another-terminal" }},
		{"foreign payment", func(_ *TBankCaptureExpectation, m map[string]any) { m["PaymentId"] = json.Number("98765") }},
		{"unknown bank error", func(_ *TBankCaptureExpectation, m map[string]any) { m["ErrorCode"] = "500" }},
		{"unexpected expectation", func(e *TBankCaptureExpectation, _ map[string]any) { e.AmountMinor = 1 }},
	}
	for _, item := range failures {
		t.Run(item.name, func(t *testing.T) {
			fields := map[string]any{"TerminalKey": "test-terminal", "OrderId": "abc_123", "PaymentId": json.Number("12345"), "Amount": json.Number("259900"), "Status": "CONFIRMED", "Success": true, "ErrorCode": "0"}
			want := expected
			item.change(&want, fields)
			raw := signedTBankCapture(t, fields)
			if _, err := VerifyTBankCapturedEvent(raw, "test-password", want); err == nil {
				t.Fatal("accepted unsafe capture")
			}
		})
	}
	if _, err := VerifyTBankCapturedEvent([]byte(`{"OrderId":"a","OrderId":"b"}`), "test-password", expected); err == nil {
		t.Fatal("accepted duplicate json fields")
	}
	if _, err := VerifyTBankCapturedEvent(append(good, []byte(" {}")...), "test-password", expected); err == nil {
		t.Fatal("accepted trailing objects")
	}
}
