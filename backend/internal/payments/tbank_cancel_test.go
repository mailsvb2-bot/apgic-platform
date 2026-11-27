package payments

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestTBankCancelUsesOriginalPaymentAndSignedTerminal(t *testing.T) {
	calls := 0
	bank, err := NewTBankAcquiring("merchant", "secret", &http.Client{Transport: tbankRoundTrip(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.String() != tbankCancelEndpoint || req.Method != http.MethodPost {
			return tbankReply(400, "bad"), nil
		}
		var payload map[string]any
		dec := json.NewDecoder(req.Body)
		dec.UseNumber()
		if dec.Decode(&payload) != nil {
			return tbankReply(400, "bad"), nil
		}
		token, _ := payload["Token"].(string)
		delete(payload, "Token")
		expected, err := TBankToken(payload, "secret")
		if err != nil || token != expected || payload["TerminalKey"] != "merchant" ||
			payload["PaymentId"] != "12345" || payload["Amount"] != json.Number("49900") {
			t.Fatalf("cancel must carry signed original PSP evidence: %#v", payload)
		}
		return tbankReply(200, `{"Success":true,"ErrorCode":"0","PaymentId":12345,"Status":"REFUNDED"}`), nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	out, err := bank.Cancel(context.Background(), TBankCancelRequest{PaymentID: "12345", AmountMinor: 49900})
	if err != nil || out.Status != "REFUNDED" || out.PaymentID != "12345" || calls != 1 {
		t.Fatalf("bank cancel result=%+v err=%v calls=%d", out, err, calls)
	}
}

func TestTBankCancelRejectsForeignProviderDataBeforeNetwork(t *testing.T) {
	calls := 0
	bank, _ := NewTBankAcquiring("merchant", "secret", &http.Client{Transport: tbankRoundTrip(func(*http.Request) (*http.Response, error) {
		calls++
		return tbankReply(200, "{}"), nil
	})})
	for _, bad := range []TBankCancelRequest{
		{PaymentID: "", AmountMinor: 100},
		{PaymentID: "../../123", AmountMinor: 100},
		{PaymentID: "123a", AmountMinor: 100},
		{PaymentID: "123", AmountMinor: 0},
		{PaymentID: "123", AmountMinor: 10000000001},
	} {
		_, err := bank.Cancel(context.Background(), bad)
		if !errors.Is(err, ErrTBankCancel) {
			t.Fatalf("accepted foreign request %#v err=%v", bad, err)
		}
	}
	if calls != 0 {
		t.Fatalf("untrusted bank data performed %d external calls", calls)
	}
}

func TestTBankCancelResponseIsNotRefundEvidence(t *testing.T) {
	for _, body := range []string{
		`{"Success":false,"ErrorCode":"99","PaymentId":123,"Status":"REFUNDED"}`,
		`{"Success":true,"ErrorCode":"0","PaymentId":124,"Status":"REFUNDED"}`,
		`{"Success":true,"ErrorCode":"0","PaymentId":123,"Status":""}`,
		`{"Success":true,"ErrorCode":"0","PaymentId":123,"PaymentId":123,"Status":"REFUNDED"}`,
		`{"Success":true,"ErrorCode":"0","PaymentId":123,"Status":"REFUNDED"}{"other":1}`,
		strings.Repeat("x", 8194),
	} {
		bank, _ := NewTBankAcquiring("merchant", "secret", &http.Client{Transport: tbankRoundTrip(func(*http.Request) (*http.Response, error) {
			return tbankReply(200, body), nil
		})})
		_, err := bank.Cancel(context.Background(), TBankCancelRequest{PaymentID: "123", AmountMinor: 100})
		if !errors.Is(err, ErrTBankCancel) {
			t.Fatalf("accepted unverified PSP reply %q", body)
		}
	}
}
