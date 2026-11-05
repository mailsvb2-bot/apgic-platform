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

func TestAcademyVideoTrustedStorageRedirects(t *testing.T) {
	client := academyVideoUpstreamClient()
	via := []*http.Request{{}}
	for raw, allowed := range map[string]bool{
		"https://s123vla.storage.yandex.net/rdisk/abc?x=1": true,
		"https://downloader.disk.yandex.ru/disk/abc":       true,
		"https://evil.example/rdisk":                       false,
		"http://s1.storage.yandex.net/rdisk":               false,
		"https://a.b.storage.yandex.net/rdisk":             false,
		"https://user@s1.storage.yandex.net/":              false,
		"https://s1.storage.yandex.net:8443/":              false,
	} {
		req, _ := http.NewRequest(http.MethodGet, raw, nil)
		req.Header.Set("Authorization", "OAuth secret")
		err := client.CheckRedirect(req, via)
		if (err == nil) != allowed {
			t.Fatalf("redirect %q allowed=%v err=%v", raw, allowed, err)
		}
		if err == nil && req.Header.Get("Authorization") != "" {
			t.Fatal("credential leaked to redirect")
		}
	}
	tooMany := []*http.Request{{}, {}, {}, {}}
	req, _ := http.NewRequest(http.MethodGet, "https://s1.storage.yandex.net/x", nil)
	if client.CheckRedirect(req, tooMany) == nil {
		t.Fatal("unbounded redirect")
	}
}
