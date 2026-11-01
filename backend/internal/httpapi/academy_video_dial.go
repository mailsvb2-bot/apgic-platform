package httpapi

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"syscall"
)

// The cloud URL host allowlist is necessary but not sufficient: a DNS answer
// must not turn a trusted hostname into a connection to a private service.
// Dialer.ControlContext receives the resolved IP immediately before connect,
// avoiding a separate lookup-and-connect DNS rebinding window.
var academyVideoNonPublicNetworks = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
}

func academyVideoPublicDownloadIP(ip netip.Addr) bool {
	if !ip.IsValid() || ip.Zone() != "" {
		return false
	}
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return false
	}
	for _, network := range academyVideoNonPublicNetworks {
		if network.Contains(ip) {
			return false
		}
	}
	return true
}

func academyVideoDialControl(_ context.Context, _, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return errors.New("invalid cloud media destination")
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !academyVideoPublicDownloadIP(ip) {
		return errors.New("non-public cloud media destination")
	}
	return nil
}
