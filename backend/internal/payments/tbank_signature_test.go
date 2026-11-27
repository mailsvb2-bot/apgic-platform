package payments

import (
	"errors"
	"testing"
)

func TestTBankTokenPublishedVector(t *testing.T) {
	token, err := TBankToken(map[string]any{
		"TerminalKey": "MerchantTerminalKey", "Amount": 19200, "OrderId": "00000", "Description": "Подарочная карта на 1000 рублей",
		"DATA": map[string]any{"Email": "private@example.com"}, "Receipt": map[string]any{"Taxation": "osn"},
	}, "11111111111111")
	if err != nil || token != "72dd466f8ace0a37a1f740ce5fb78101712bc0665d91a8108c7c8a0ccd426db2" {
		t.Fatalf("token %q err %v", token, err)
	}
}

func TestTBankNotificationMustVerify(t *testing.T) {
	event := map[string]any{"TerminalKey": "TBankTest", "OrderId": "course-1", "PaymentId": "123", "Amount": 19900, "Success": true, "Status": "CONFIRMED", "Data": map[string]any{"Never": "sign"}}
	token, err := TBankToken(event, "secret")
	if err != nil {
		t.Fatal(err)
	}
	event["Token"] = token
	if err := VerifyTBankNotificationToken(event, "secret"); err != nil {
		t.Fatal(err)
	}
	event["Amount"] = 19901
	if err := VerifyTBankNotificationToken(event, "secret"); !errors.Is(err, ErrTBankSignature) {
		t.Fatalf("tampered notification accepted: %v", err)
	}
	event["Amount"] = 19900
	if err := VerifyTBankNotificationToken(event, "wrong-secret"); !errors.Is(err, ErrTBankSignature) {
		t.Fatalf("wrong secret accepted: %v", err)
	}
	event["Token"] = "not-hex"
	if err := VerifyTBankNotificationToken(event, "secret"); !errors.Is(err, ErrTBankSignature) {
		t.Fatalf("bad signature accepted: %v", err)
	}
}

func TestTBankRejectsUnknownRootObjectsAndNoPassword(t *testing.T) {
	for _, fields := range []map[string]any{{"OrderId": map[string]any{"bad": "data"}}, {"OrderId": []string{"bad"}}, {"Password": "injected"}} {
		if _, err := TBankToken(fields, "secret"); err == nil {
			t.Fatalf("accepted %v", fields)
		}
	}
	if _, err := TBankToken(map[string]any{"OrderId": "1"}, ""); err == nil {
		t.Fatal("accepted empty secret")
	}
}
