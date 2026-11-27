package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const tbankInitEndpoint = "https://securepay.tinkoff.ru/v2/Init"

var ErrTBankInit = errors.New("tbank acquiring initiation unavailable")

var tbankOrderPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// TBankInitRequest must originate from a durable canonical Commerce order.
// The client does not create orders, attempts, captures or Entitlements.
type TBankInitRequest struct {
	OrderID         string
	AmountMinor     int64
	Description     string
	Receipt         json.RawMessage
	NotificationURL string
}

type TBankInitResult struct {
	PaymentID  string
	PaymentURL string
	Status     string
}

// TBankAcquiring only performs the external PSP execution. The caller must
// first persist a canonical order and CREATED payment attempt transactionally,
// then persist the provider reference before exposing the bank redirect.
type TBankAcquiring struct {
	terminalKey string
	password    string
	client      *http.Client
}

func NewTBankAcquiring(terminalKey, password string, client *http.Client) (*TBankAcquiring, error) {
	if terminalKey == "" || password == "" || strings.TrimSpace(terminalKey) != terminalKey || len(terminalKey) > 128 || len(password) > 256 {
		return nil, ErrTBankInit
	}
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	safe := *client
	safe.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if safe.Timeout == 0 || safe.Timeout > 15*time.Second {
		safe.Timeout = 12 * time.Second
	}
	return &TBankAcquiring{terminalKey: terminalKey, password: password, client: &safe}, nil
}

// TerminalKey is public PSP routing identity, never the secret password.
func (a *TBankAcquiring) TerminalKey() string {
	if a == nil {
		return ""
	}
	return a.terminalKey
}

// Bank returns a payment form URL only. Success=true is NOT a captured payment.
func (a *TBankAcquiring) Init(ctx context.Context, order TBankInitRequest) (TBankInitResult, error) {
	if a == nil || a.client == nil || ctx == nil || !tbankOrderPattern.MatchString(order.OrderID) ||
		order.AmountMinor <= 0 || order.AmountMinor > 10000000000 ||
		len(order.Description) == 0 || len(order.Description) > 140 ||
		len(order.Receipt) == 0 || len(order.Receipt) > 8192 || !json.Valid(order.Receipt) ||
		strings.ContainsAny(order.Description, "\r\n") {
		return TBankInitResult{}, ErrTBankInit
	}
	var receipt map[string]any
	if json.Unmarshal(order.Receipt, &receipt) != nil || len(receipt) == 0 ||
		!validateTBankReceipt(order.Receipt, order.AmountMinor) {
		return TBankInitResult{}, ErrTBankInit
	}
	callback, err := url.Parse(order.NotificationURL)
	if err != nil || callback.Scheme != "https" || callback.Host != "apgic.ru" ||
		callback.User != nil || callback.RawQuery != "" || callback.Fragment != "" ||
		callback.Path != "/v1/payments/tbank/notification" {
		return TBankInitResult{}, ErrTBankInit
	}
	fields := map[string]any{
		"TerminalKey": a.terminalKey, "Amount": order.AmountMinor, "OrderId": order.OrderID,
		"Description": order.Description, "NotificationURL": order.NotificationURL,
		"Receipt": receipt,
	}
	signature, err := TBankToken(fields, a.password)
	if err != nil {
		return TBankInitResult{}, ErrTBankInit
	}
	fields["Token"] = signature
	payload, err := json.Marshal(fields)
	if err != nil {
		return TBankInitResult{}, ErrTBankInit
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tbankInitEndpoint, bytes.NewReader(payload))
	if err != nil {
		return TBankInitResult{}, ErrTBankInit
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return TBankInitResult{}, ErrTBankInit
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return TBankInitResult{}, ErrTBankInit
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8193))
	if err != nil || len(raw) > 8192 {
		return TBankInitResult{}, ErrTBankInit
	}
	var out struct {
		Success    bool        `json:"Success"`
		ErrorCode  string      `json:"ErrorCode"`
		PaymentID  json.Number `json:"PaymentId"`
		PaymentURL string      `json:"PaymentURL"`
		Status     string      `json:"Status"`
		OrderID    string      `json:"OrderId"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&out) != nil || !out.Success || out.ErrorCode != "0" ||
		out.PaymentID == "" || len(out.PaymentID) > 32 ||
		out.PaymentURL == "" || out.OrderID != order.OrderID ||
		out.Status != "NEW" {
		return TBankInitResult{}, ErrTBankInit
	}
	for _, digit := range out.PaymentID {
		if digit < '0' || digit > '9' {
			return TBankInitResult{}, ErrTBankInit
		}
	}
	paymentURL, err := url.Parse(out.PaymentURL)
	if err != nil || paymentURL.Scheme != "https" || paymentURL.User != nil ||
		paymentURL.Fragment != "" || paymentURL.Hostname() == "" || paymentURL.Port() != "" ||
		!(paymentURL.Hostname() == "tbank.ru" || strings.HasSuffix(paymentURL.Hostname(), ".tbank.ru") ||
			paymentURL.Hostname() == "tinkoff.ru" || strings.HasSuffix(paymentURL.Hostname(), ".tinkoff.ru")) {
		return TBankInitResult{}, ErrTBankInit
	}
	return TBankInitResult{PaymentID: string(out.PaymentID), PaymentURL: out.PaymentURL, Status: out.Status}, nil
}
