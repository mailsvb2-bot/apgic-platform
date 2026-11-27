package payments

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestTBankFullRefundNeedsIndependentSignedNotificationAndState(t *testing.T) {
	expected := TBankCaptureExpectation{TerminalKey: "merchant", OrderID: "academy123", PaymentID: "234567", AmountMinor: 49900}
	fields := map[string]any{
		"TerminalKey": "merchant", "OrderId": "academy123",
		"PaymentId": json.Number("234567"), "Amount": json.Number("49900"),
		"Status": "REFUNDED", "Success": true, "ErrorCode": "0",
	}
	signed := signedTBankCapture(t, fields)
	readbacks := 0
	state := `{"TerminalKey":"merchant","OrderId":"academy123","PaymentId":234567,"Amount":49900,"Status":"REFUNDED","Success":true,"ErrorCode":"0"}`
	bank, err := NewTBankAcquiring("merchant", "test-password", &http.Client{Transport: tbankRoundTrip(func(req *http.Request) (*http.Response, error) {
		readbacks++
		if req.URL.String() != tbankGetStateEndpoint {
			t.Fatal("wrong bank endpoint")
		}
		return tbankReply(http.StatusOK, state), nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	result, err := bank.ConfirmRefundNotificationWithGetState(context.Background(), signed, expected)
	if err != nil || readbacks != 1 || result.ProviderStatus != "REFUNDED" {
		t.Fatalf("full refund rejected: %+v %v reads=%d", result, err, readbacks)
	}
	// Even an authentic bank REFUNDED notification cannot become a capture.
	if _, err = VerifyTBankCapturedEvent(signed, "test-password", expected); err == nil {
		t.Fatal("refund notification was admitted as payment capture")
	}
	state = `{"TerminalKey":"merchant","OrderId":"academy123","PaymentId":234567,"Amount":49900,"Status":"PARTIAL_REFUNDED","Success":true,"ErrorCode":"0"}`
	if _, err = bank.ConfirmRefundNotificationWithGetState(context.Background(), signed, expected); err == nil {
		t.Fatal("partial refund revoked entire purchase")
	}
	state = `{"TerminalKey":"merchant","OrderId":"academy123","PaymentId":234567,"Amount":49900,"Status":"CONFIRMED","Success":true,"ErrorCode":"0"}`
	if _, err = bank.ConfirmRefundNotificationWithGetState(context.Background(), signed, expected); err == nil {
		t.Fatal("Cancel request succeeded but original purchase was still confirmed")
	}
	state = `{"TerminalKey":"merchant","OrderId":"other","PaymentId":234567,"Amount":49900,"Status":"REFUNDED","Success":true,"ErrorCode":"0"}`
	if _, err = bank.ConfirmRefundNotificationWithGetState(context.Background(), signed, expected); err == nil {
		t.Fatal("accepted foreign order refund")
	}
	state = `{"TerminalKey":"merchant","OrderId":"academy123","PaymentId":234567,"Amount":49900,"Status":"REFUNDED","Status":"CONFIRMED","Success":true,"ErrorCode":"0"}`
	if _, err = bank.ConfirmRefundNotificationWithGetState(context.Background(), signed, expected); err == nil {
		t.Fatal("accepted duplicate status from bank")
	}
	reads := readbacks
	if _, err = bank.ConfirmRefundNotificationWithGetState(context.Background(), []byte(`{}`), expected); err == nil || readbacks != reads {
		t.Fatal("unsigned refund must not reach bank or release entitlement")
	}
}

func TestTBankFullRefundWrongSellerNeverContactsBank(t *testing.T) {
	bank, _ := NewTBankAcquiring("merchant", "test-password", &http.Client{Transport: tbankRoundTrip(func(*http.Request) (*http.Response, error) {
		t.Fatal("foreign seller cannot query own bank as another terminal")
		return nil, nil
	})})
	expected := TBankCaptureExpectation{TerminalKey: "another-owner", OrderID: "academy123", PaymentID: "234567", AmountMinor: 49900}
	if _, err := bank.ConfirmRefundNotificationWithGetState(context.Background(), []byte(`{}`), expected); err == nil {
		t.Fatal("foreign terminal admitted")
	}
}
