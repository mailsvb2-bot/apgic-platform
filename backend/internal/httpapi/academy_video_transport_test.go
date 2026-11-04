package httpapi

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

// An authorized video can stream for minutes, but a CDN endpoint that never
// completes TLS or floods headers must not pin a gateway worker indefinitely.
// The upstream transport also must not inherit environment proxies or follow
// a redirect to an untrusted host.
func TestAcademyVideoUpstreamClientTransportLimits(t *testing.T) {
	client := academyVideoUpstreamClient()
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("media gateway must use a dedicated HTTP transport")
	}
	if transport.Proxy != nil || !transport.DisableCompression || !transport.DisableKeepAlives {
		t.Fatal("media transport must reject proxies, compression and connection reuse")
	}
	if transport.DialContext == nil {
		t.Fatal("media transport must retain its SSRF-protected dialer")
	}
	if transport.TLSHandshakeTimeout <= 0 || transport.TLSHandshakeTimeout > 10*time.Second {
		t.Fatalf("unbounded media TLS handshake: %s", transport.TLSHandshakeTimeout)
	}
	if transport.ResponseHeaderTimeout <= 0 || transport.ResponseHeaderTimeout > 15*time.Second {
		t.Fatalf("unbounded media response headers: %s", transport.ResponseHeaderTimeout)
	}
	if transport.MaxResponseHeaderBytes <= 0 || transport.MaxResponseHeaderBytes > 64<<10 {
		t.Fatalf("unbounded media response header bytes: %d", transport.MaxResponseHeaderBytes)
	}
	if client.Timeout != 0 {
		t.Fatal("whole-response timeout would truncate authorized long-running video")
	}
	if client.CheckRedirect == nil || !errors.Is(client.CheckRedirect(&http.Request{}, nil), http.ErrUseLastResponse) {
		t.Fatal("media transport must reject missing trusted redirect hop")
	}
}
