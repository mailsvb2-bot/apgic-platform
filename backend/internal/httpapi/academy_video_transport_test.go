package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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
	t.Setenv("APGIC_YANDEX_STORAGE_HOSTS", "s123vla.storage.yandex.net,s1.storage.yandex.net")
	client := academyVideoUpstreamClient()
	trustedOrigin, err := http.NewRequest(http.MethodGet, "https://downloader.disk.yandex.net/rdisk/source", nil)
	if err != nil {
		t.Fatal(err)
	}
	via := []*http.Request{trustedOrigin}
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
	tooMany := []*http.Request{trustedOrigin, trustedOrigin, trustedOrigin, trustedOrigin}
	req, _ := http.NewRequest(http.MethodGet, "https://s1.storage.yandex.net/x", nil)
	if client.CheckRedirect(req, tooMany) == nil {
		t.Fatal("unbounded redirect")
	}
}

// Direct Yandex delivery uses the same authenticated Academy identity
// boundary as every private learner route. A missing signed session must
// fail before any PostgreSQL credential lookup or provider network I/O.
func TestAcademyProductionVideoRouteFailsClosedWithoutSignedSession(t *testing.T) {
	mux := http.NewServeMux()
	registerAcademyVideoStream(mux, nil, nil, nil)

	request := httptest.NewRequest(http.MethodGet,
		"/v1/academy/enrollments/00000000-0000-0000-0000-000000000001/lessons/00000000-0000-0000-0000-000000000002/videos/00000000-0000-0000-0000-000000000003/stream",
		nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("production video route returned HTTP %d, want 503 without a signed session", response.Code)
	}
	if response.Header().Get("Location") != "" {
		t.Fatalf("production video route exposed a redirect: %q", response.Header().Get("Location"))
	}
	for _, forbidden := range []string{"downloader.disk.yandex", "storage.yandex.net", "OAuth ", "disk:/"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("production video route disclosed origin detail %q", forbidden)
		}
	}
	if !strings.Contains(response.Body.String(), "ACADEMY_SESSION_UNAVAILABLE") {
		t.Fatalf("production video route did not return the canonical session unavailable error: %s", response.Body.String())
	}
}
