package payments

import (
	"bytes"
	"encoding/json"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// validateTBankReceipt checks the fiscal math before initiating an external
// charge. Taxation and the payment object are supplied from the seller's
// approved legal/fiscal configuration, never invented from course metadata.
func validateTBankReceipt(raw json.RawMessage, orderAmountMinor int64) bool {
	if len(raw) == 0 || len(raw) > 8192 || orderAmountMinor <= 0 {
		return false
	}
	var receipt struct {
		Taxation string `json:"Taxation"`
		Items    []struct {
			Name          string      `json:"Name"`
			Price         json.Number `json:"Price"`
			Quantity      json.Number `json:"Quantity"`
			Amount        json.Number `json:"Amount"`
			Tax           string      `json:"Tax"`
			PaymentObject string      `json:"PaymentObject"`
		} `json:"Items"`
		Payments *struct {
			Electronic     json.Number `json:"Electronic"`
			Cash           json.Number `json:"Cash"`
			AdvancePayment json.Number `json:"AdvancePayment"`
			Credit         json.Number `json:"Credit"`
			Provision      json.Number `json:"Provision"`
		} `json:"Payments"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&receipt) != nil || receipt.Taxation == "" ||
		len(receipt.Items) == 0 || len(receipt.Items) > 100 {
		return false
	}
	switch receipt.Taxation {
	case "osn", "usn_income", "usn_income_outcome", "esn", "patent":
	default:
		return false
	}
	var sum int64
	for _, item := range receipt.Items {
		if strings.TrimSpace(item.Name) == "" || len([]rune(item.Name)) > 128 ||
			strings.TrimSpace(item.PaymentObject) == "" || item.Tax == "" {
			return false
		}
		price, priceErr := item.Price.Int64()
		amount, amountErr := item.Amount.Int64()
		qty, qtyErr := strconv.ParseFloat(string(item.Quantity), 64)
		if priceErr != nil || amountErr != nil || qtyErr != nil ||
			price <= 0 || amount <= 0 || qty <= 0 ||
			math.IsNaN(qty) || math.IsInf(qty, 0) ||
			qty > 99999.999999 ||
			amount > orderAmountMinor-sum {
			return false
		}
		quantity, ok := new(big.Rat).SetString(string(item.Quantity))
		if !ok || quantity.Sign() <= 0 {
			return false
		}
		product := new(big.Rat).Mul(new(big.Rat).SetInt64(price), quantity)
		if !product.IsInt() || product.Num().Cmp(big.NewInt(amount)) != 0 {
			return false
		}
		switch item.Tax {
		case "none", "vat0", "vat5", "vat7", "vat10", "vat22", "vat105", "vat107", "vat110", "vat122":
		default:
			return false
		}
		sum += amount
	}
	if sum != orderAmountMinor {
		return false
	}
	if receipt.Payments != nil {
		electronic, err := receipt.Payments.Electronic.Int64()
		if err != nil || electronic != orderAmountMinor {
			return false
		}
		for _, part := range []json.Number{
			receipt.Payments.Cash, receipt.Payments.AdvancePayment,
			receipt.Payments.Credit, receipt.Payments.Provision,
		} {
			if part != "" && part != "0" {
				return false
			}
		}
	}
	return true
}
