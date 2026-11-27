package payments

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTBankScopedCACertificateRequired(t *testing.T) {
	if _, err := NewTBankAcquiringWithCABundle("terminal", "password", nil); err == nil {
		t.Fatal("accepted absent trust anchor")
	}
	if _, err := NewTBankAcquiringWithCABundle("terminal", "password", []byte("untrusted junk")); err == nil {
		t.Fatal("accepted invalid certificate")
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	raw := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	client, err := NewTBankAcquiringWithCABundle("terminal", "password", raw)
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := client.client.Transport.(*http.Transport)
	if !ok || transport.TLSClientConfig == nil || transport.TLSClientConfig.RootCAs == nil ||
		transport.TLSClientConfig.InsecureSkipVerify || transport.Proxy != nil {
		t.Fatal("TLS or bank-specific proxy isolation is unsafe")
	}
}
