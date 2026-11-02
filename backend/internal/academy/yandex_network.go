package academy

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

// All Yandex OAuth, metadata and media connections share one outbound IP
// policy. A trusted hostname alone cannot prevent DNS rebinding to a local
// service; Dialer.ControlContext checks the resolved address at connect time.
var yandexNonPublicNetworks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	// netip.IsGlobalUnicast alone still accepts deprecated IPv6 site-local,
	// discard-only and special-purpose benchmarking/ORCHID destinations.
	// Such destinations must never receive OAuth credentials or media requests.
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001:2::/48"),
	netip.MustParsePrefix("2001:10::/28"),
	netip.MustParsePrefix("2001:20::/28"),
	netip.MustParsePrefix("fec0::/10"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
}

// YandexPublicIP rejects local, reserved, documentation and translated
// destinations, including IPv4-mapped and IPv4-embedded IPv6 addresses.
func YandexPublicIP(ip netip.Addr) bool {
	if !ip.IsValid() || ip.Zone() != "" {
		return false
	}
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return false
	}
	for _, network := range yandexNonPublicNetworks {
		if network.Contains(ip) {
			return false
		}
	}
	return true
}

// YandexDialControl runs on the resolved IP before a TCP connection is made.
func YandexDialControl(_ context.Context, _, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return errors.New("invalid cloud media destination")
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !YandexPublicIP(ip) {
		return errors.New("non-public cloud media destination")
	}
	return nil
}

// NewYandexSecureClient is the production transport for OAuth and Disk API
// credentials. In particular, do not inherit HTTP(S)_PROXY from the runner
// environment: it would receive the authorization code or OAuth token.
// Tests may inject a separate client into YandexDiskClient explicitly.
func NewYandexSecureClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = (&net.Dialer{
		Timeout:        10 * time.Second,
		ControlContext: YandexDialControl,
	}).DialContext
	transport.DisableCompression = true
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
