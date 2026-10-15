package runtimepostgres

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestFinanceProviderPayloadMustBeOneCompleteSignedEconomicEvent(t *testing.T) {
	valid := `{"evidence_kind":"PAYOUT_EXECUTED","settlement_instruction_id":"00000000-0000-0000-0000-000000000001","provider_reference":"bank/receipt/1","gross_amount_minor":18000,"provider_fee_minor":0,"currency":"RUB"}`
	for _, tc := range []struct {
		name string
		raw  string
		ok   bool
	}{
		{"valid", valid, true},
		{"whitespace", "  " + valid + "\n", true},
		{"second document", valid + `{"evidence_kind":"DISPUTE_REFUND"}`, false},
		{"trailing garbage", valid + " ;", false},
		{"unknown additional directive", `{"evidence_kind":"PAYOUT_EXECUTED","transfer_all_funds":true}`, false},
		{"wrong numeric type", `{"gross_amount_minor":"18000"}`, false},
		{"incomplete", `{"evidence_kind":"PAYOUT_EXECUTED"`, false},
		{"array instead of record", `[{"evidence_kind":"PAYOUT_EXECUTED"}]`, false},
		{"null", "null", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := decodeFinanceProviderPayload(json.RawMessage(tc.raw))
			if tc.ok {
				if err != nil || parsed.EvidenceKind != "PAYOUT_EXECUTED" ||
					parsed.GrossAmountMinor != 18000 || parsed.ProviderReference != "bank/receipt/1" {
					t.Fatalf("signed finance record rejected or altered: %+v %v", parsed, err)
				}
			} else if !errors.Is(err, ErrFinanceEvidenceInvalid) {
				t.Fatalf("malformed provider payload must fail closed: %+v %v", parsed, err)
			}
		})
	}
}
