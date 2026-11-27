package payments

import (
	"encoding/json"
	"testing"
)

func TestTBankFiscalReceiptMatchesExactEconomics(t *testing.T) {
	good := json.RawMessage(`{"Taxation":"usn_income","Items":[{"Name":"Academy course","Price":500,"Quantity":2,"Amount":1000,"Tax":"none","PaymentObject":"service"}],"Payments":{"Electronic":1000}}`)
	if !validateTBankReceipt(good, 1000) {
		t.Fatal("valid receipt refused")
	}
	fraction := json.RawMessage(`{"Taxation":"usn_income","Items":[{"Name":"Service","Price":200,"Quantity":0.5,"Amount":100,"Tax":"none","PaymentObject":"service"}]}`)
	if !validateTBankReceipt(fraction, 100) {
		t.Fatal("exact fractional quantity rejected")
	}
	if validateTBankReceipt(good, 1001) {
		t.Fatal("accepted mismatched order amount")
	}
}

func TestTBankFiscalReceiptBlocksBogusSellerAndItemMath(t *testing.T) {
	rows := map[string]string{
		"missing taxation":       `{"Items":[{"Name":"X","Price":500,"Quantity":2,"Amount":1000,"Tax":"none","PaymentObject":"service"}]}`,
		"unknown taxation":       `{"Taxation":"foreign","Items":[{"Name":"X","Price":500,"Quantity":2,"Amount":1000,"Tax":"none","PaymentObject":"service"}]}`,
		"missing payment object": `{"Taxation":"usn_income","Items":[{"Name":"X","Price":500,"Quantity":2,"Amount":1000,"Tax":"none"}]}`,
		"missing tax":            `{"Taxation":"usn_income","Items":[{"Name":"X","Price":500,"Quantity":2,"Amount":1000,"PaymentObject":"service"}]}`,
		"wrong multiplication":   `{"Taxation":"usn_income","Items":[{"Name":"X","Price":501,"Quantity":2,"Amount":1000,"Tax":"none","PaymentObject":"service"}]}`,
		"zero quantity":          `{"Taxation":"usn_income","Items":[{"Name":"X","Price":500,"Quantity":0,"Amount":1000,"Tax":"none","PaymentObject":"service"}]}`,
		"wrong payment totals":   `{"Taxation":"usn_income","Items":[{"Name":"X","Price":500,"Quantity":2,"Amount":1000,"Tax":"none","PaymentObject":"service"}],"Payments":{"Electronic":999}}`,
		"cash mixing":            `{"Taxation":"usn_income","Items":[{"Name":"X","Price":500,"Quantity":2,"Amount":1000,"Tax":"none","PaymentObject":"service"}],"Payments":{"Electronic":1000,"Cash":1000}}`,
		"wrong vat":              `{"Taxation":"usn_income","Items":[{"Name":"X","Price":500,"Quantity":2,"Amount":1000,"Tax":"wrong","PaymentObject":"service"}]}`,
		"overflow":               `{"Taxation":"usn_income","Items":[{"Name":"X","Price":9223372036854775807,"Quantity":2,"Amount":1000,"Tax":"none","PaymentObject":"service"}]}`,
		"malformed json":         `{"Taxation":`,
	}
	for name, payload := range rows {
		t.Run(name, func(t *testing.T) {
			if validateTBankReceipt(json.RawMessage(payload), 1000) {
				t.Fatal("bad fiscal receipt accepted")
			}
		})
	}
}
