package payments

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

var ErrTBankCapture = errors.New("tbank capture is unverified")

// TBankCaptureExpectation comes from a persisted canonical Commerce
// order and payment attempt, never a browser or webhook-provided quote.
type TBankCaptureExpectation struct {
	TerminalKey string
	OrderID     string
	PaymentID   string
	AmountMinor int64
}

type TBankCapturedEvent struct {
	OrderID        string
	PaymentID      string
	AmountMinor    int64
	ProviderStatus string
}

// VerifyTBankCapturedEvent checks the signed webhook against the original
// persisted bank attempt. It cannot issue rights, ledger entries, or receipts.
// Only the existing Commerce transaction owner may commit those effects.
func VerifyTBankCapturedEvent(raw []byte, password string, expected TBankCaptureExpectation) (TBankCapturedEvent, error) {
	if len(raw) == 0 || len(raw) > 16384 || expected.TerminalKey == "" ||
		!tbankOrderPattern.MatchString(expected.OrderID) || expected.PaymentID == "" ||
		expected.AmountMinor <= 0 {
		return TBankCapturedEvent{}, ErrTBankCapture
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return TBankCapturedEvent{}, ErrTBankCapture
	}
	fields := make(map[string]any)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return TBankCapturedEvent{}, ErrTBankCapture
		}
		key, ok := token.(string)
		if !ok {
			return TBankCapturedEvent{}, ErrTBankCapture
		}
		if _, exists := fields[key]; exists {
			return TBankCapturedEvent{}, ErrTBankCapture
		}
		var value any
		if decoder.Decode(&value) != nil {
			return TBankCapturedEvent{}, ErrTBankCapture
		}
		fields[key] = value
	}
	if end, err := decoder.Token(); err != nil || end != json.Delim('}') {
		return TBankCapturedEvent{}, ErrTBankCapture
	}
	if _, err := decoder.Token(); err != io.EOF {
		return TBankCapturedEvent{}, ErrTBankCapture
	}
	if err := VerifyTBankNotificationToken(fields, password); err != nil {
		return TBankCapturedEvent{}, ErrTBankCapture
	}
	amount, amountOK := fields["Amount"].(json.Number)
	paymentID, paymentOK := fields["PaymentId"].(json.Number)
	orderID, orderOK := fields["OrderId"].(string)
	terminal, terminalOK := fields["TerminalKey"].(string)
	status, statusOK := fields["Status"].(string)
	success, successOK := fields["Success"].(bool)
	errorCode, codeOK := fields["ErrorCode"].(string)
	amountValue, amountErr := amount.Int64()
	if !amountOK || !paymentOK || !orderOK || !terminalOK || !statusOK || !successOK || !codeOK ||
		amountErr != nil || amountValue != expected.AmountMinor || orderID != expected.OrderID ||
		terminal != expected.TerminalKey || string(paymentID) != expected.PaymentID ||
		!success || errorCode != "0" || status != "CONFIRMED" {
		return TBankCapturedEvent{}, ErrTBankCapture
	}
	return TBankCapturedEvent{OrderID: orderID, PaymentID: string(paymentID), AmountMinor: amountValue, ProviderStatus: status}, nil
}

// VerifyCapturedNotification validates bank-origin evidence without any
// network operation. Separate from GetState so callers can distinguish a
// forged notification (reject) from a transient bank outage (retry).
func (a *TBankAcquiring) VerifyCapturedNotification(raw []byte,
 expected TBankCaptureExpectation) (TBankCapturedEvent,error) {
 if a==nil||expected.TerminalKey!=a.terminalKey{
  return TBankCapturedEvent{},ErrTBankCapture
 }
 return VerifyTBankCapturedEvent(raw,a.password,expected)
}
