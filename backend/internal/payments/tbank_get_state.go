package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
)

const tbankGetStateEndpoint = "https://securepay.tinkoff.ru/v2/GetState"

// GetState performs a second read-only provider check of a previously
// persisted canonical Commerce payment attempt. Only CONFIRMED can be
// returned as capture evidence; this method never updates the ledger or
// issues an Entitlement.
func (a *TBankAcquiring) GetState(ctx context.Context, expected TBankCaptureExpectation) (TBankCapturedEvent, error) {
	return a.getStateWithRequiredStatus(ctx, expected, "CONFIRMED")
}

// GetRefundedState accepts only a full settled REFUNDED bank status for the
// persisted original order/attempt. A Cancel response is NOT sufficient.
func (a *TBankAcquiring) GetRefundedState(ctx context.Context, expected TBankCaptureExpectation) (TBankCapturedEvent, error) {
	return a.getStateWithRequiredStatus(ctx, expected, "REFUNDED")
}

func (a *TBankAcquiring) getStateWithRequiredStatus(ctx context.Context, expected TBankCaptureExpectation, requiredStatus string) (TBankCapturedEvent, error) {
	if a == nil || a.client == nil || ctx == nil ||
		expected.TerminalKey != a.terminalKey ||
		!tbankOrderPattern.MatchString(expected.OrderID) ||
		expected.AmountMinor <= 0 || expected.AmountMinor > 10000000000 ||
		expected.PaymentID == "" || len(expected.PaymentID) > 32 {
		return TBankCapturedEvent{}, ErrTBankCapture
	}
	if _, err := strconv.ParseUint(expected.PaymentID, 10, 64); err != nil {
		return TBankCapturedEvent{}, ErrTBankCapture
	}
	fields := map[string]any{"TerminalKey": a.terminalKey, "PaymentId": expected.PaymentID}
	token, err := TBankToken(fields, a.password)
	if err != nil {
		return TBankCapturedEvent{}, ErrTBankCapture
	}
	fields["Token"] = token
	payload, err := json.Marshal(fields)
	if err != nil {
		return TBankCapturedEvent{}, ErrTBankCapture
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tbankGetStateEndpoint, bytes.NewReader(payload))
	if err != nil {
		return TBankCapturedEvent{}, ErrTBankCapture
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return TBankCapturedEvent{}, ErrTBankCapture
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return TBankCapturedEvent{}, ErrTBankCapture
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16385))
	if err != nil || len(raw) > 16384 {
		return TBankCapturedEvent{}, ErrTBankCapture
	}
	// Reject duplicate top-level fields before decoding typed economic facts.
	// A provider response containing conflicting OrderId/Amount/Status values
	// must never become an independent confirmation of a signed webhook.
	if !tbankUniqueRootFields(raw) {
		return TBankCapturedEvent{}, ErrTBankCapture
	}
	// Bank GetState is signed as a REQUEST; its response is separately
	// validated for all economic and identity fields. A status response
	// without an independent signed notification is not sufficient for
	// granting paid Academy rights.
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value struct {
		TerminalKey string      `json:"TerminalKey"`
		OrderID     string      `json:"OrderId"`
		PaymentID   json.Number `json:"PaymentId"`
		Amount      json.Number `json:"Amount"`
		Status      string      `json:"Status"`
		Success     bool        `json:"Success"`
		ErrorCode   string      `json:"ErrorCode"`
	}
	if decoder.Decode(&value) != nil {
		return TBankCapturedEvent{}, ErrTBankCapture
	}
	if _, err := decoder.Token(); err != io.EOF {
		return TBankCapturedEvent{}, ErrTBankCapture
	}
	amount, err := value.Amount.Int64()
	if err != nil || !value.Success || value.ErrorCode != "0" ||
		value.Status != requiredStatus || value.TerminalKey != expected.TerminalKey ||
		value.OrderID != expected.OrderID || string(value.PaymentID) != expected.PaymentID ||
		amount != expected.AmountMinor {
		return TBankCapturedEvent{}, ErrTBankCapture
	}
	return TBankCapturedEvent{
		OrderID: value.OrderID, PaymentID: string(value.PaymentID),
		AmountMinor: amount, ProviderStatus: value.Status,
	}, nil
}

// ConfirmNotificationWithGetState requires two independently obtained pieces
// of bank evidence: a signed CONFIRMED webhook and a fresh authenticated
// GetState read. It remains a precondition for the canonical Commerce writer,
// not an alternative payment ledger or a grant operation.
func (a *TBankAcquiring) ConfirmNotificationWithGetState(
	ctx context.Context, signedNotification []byte, expected TBankCaptureExpectation,
) (TBankCapturedEvent, error) {
	if a == nil || expected.TerminalKey != a.terminalKey {
		return TBankCapturedEvent{}, ErrTBankCapture
	}
	webhook, err := VerifyTBankCapturedEvent(signedNotification, a.password, expected)
	if err != nil {
		return TBankCapturedEvent{}, ErrTBankCapture
	}
	state, err := a.GetState(ctx, expected)
	if err != nil || state != webhook {
		return TBankCapturedEvent{}, ErrTBankCapture
	}
	return webhook, nil
}

// Bank response must be a single JSON object with unique root keys.
// Nested data may be provider-specific; it is never used for payment facts.
func tbankUniqueRootFields(raw []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return false
	}
	seen := make(map[string]struct{})
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		key, ok := token.(string)
		if !ok {
			return false
		}
		if _, exists := seen[key]; exists {
			return false
		}
		seen[key] = struct{}{}
		var field json.RawMessage
		if decoder.Decode(&field) != nil {
			return false
		}
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') {
		return false
	}
	_, err = decoder.Token()
	return err == io.EOF
}
