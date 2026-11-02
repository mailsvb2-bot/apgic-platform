package academy

import (
	"context"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestYandexSecureClientDoesNotInheritEnvironmentProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:3128")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:3128")
	client := NewYandexSecureClient(12 * time.Second)
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.Proxy != nil || transport.DialContext == nil {
		t.Fatal("credential-bearing Yandex client must use a direct guarded transport")
	}
	if client.Timeout != 12*time.Second {
		t.Fatal("Yandex request timeout was lost")
	}
	if client.CheckRedirect == nil {
		t.Fatal("OAuth credential transport must reject redirects")
	}
	_, err := client.Get("http://127.0.0.1:1/private")
	if err == nil || !strings.Contains(err.Error(), "non-public cloud media destination") {
		t.Fatalf("DNS-rebound private destination was not rejected: %v", err)
	}
}

func TestYandexSharedOutboundPolicyBlocksNonPublicIPs(t *testing.T) {
	for _, value := range []string{
		"127.0.0.1", "::1", "10.0.0.1", "169.254.169.254",
		"::ffff:127.0.0.1", "100.64.1.1", "192.0.2.1",
		"2001:db8::1", "2002:c0a8:101::", "64:ff9b::a9fe:a9fe",
	} {
		if YandexPublicIP(netip.MustParseAddr(value)) {
			t.Fatalf("non-public Yandex destination allowed: %s", value)
		}
	}
	for _, value := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if !YandexPublicIP(netip.MustParseAddr(value)) {
			t.Fatalf("public destination rejected: %s", value)
		}
	}
	for _, address := range []string{"127.0.0.1:443", "[::1]:443", "cloud-api.yandex.net:443"} {
		if err := YandexDialControl(context.Background(), "tcp", address, nil); err == nil {
			t.Fatalf("unsafe resolved address allowed: %s", address)
		}
	}
}
