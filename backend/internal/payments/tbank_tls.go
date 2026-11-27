package payments

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"time"
)

// NewTBankAcquiringWithCABundle adds explicitly provisioned Russian Trusted
// CA PEM roots to the SYSTEM pool, only for this bank's HTTP client. This
// never disables certificate or hostname verification and never modifies
// the OS trust store. The operator must verify the CA source out of band.
// No certificate or private key should come from an HTTP request.
func NewTBankAcquiringWithCABundle(terminalKey, password string, caBundlePEM []byte) (*TBankAcquiring, error) {
	if len(caBundlePEM) == 0 || len(caBundlePEM) > 262144 {
		return nil, ErrTBankInit
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		return nil, ErrTBankInit
	}
	if !pool.AppendCertsFromPEM(caBundlePEM) {
		return nil, ErrTBankInit
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}
	return NewTBankAcquiring(terminalKey, password, &http.Client{Transport: transport, Timeout: 12 * time.Second})
}
