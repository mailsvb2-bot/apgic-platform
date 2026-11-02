package httpapi

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestAcademyVideoPublicDownloadIP(t *testing.T) {
	for _, addr := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if !academyVideoPublicDownloadIP(netip.MustParseAddr(addr)) {
			t.Fatalf("public IP rejected: %s", addr)
		}
	}
	for _, addr := range []string{
		"0.0.0.1", "0.1.2.3", "::ffff:0.0.0.1",
		"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1",
		"169.254.169.254", "100.64.1.1", "192.0.2.1", "198.18.0.1",
		"203.0.113.1", "240.0.0.1", "::1", "fc00::1", "fe80::1",
		"::ffff:127.0.0.1", "2001:db8::1", "2002:c0a8:101::",
		"64:ff9b::a9fe:a9fe", "fe80::1%eth0",
	} {
		if academyVideoPublicDownloadIP(netip.MustParseAddr(addr)) {
			t.Fatalf("non-public IP accepted: %s", addr)
		}
	}
}

func TestAcademyVideoDialControlBlocksPrivateResolvedAddresses(t *testing.T) {
	for _, address := range []string{
		"0.0.0.1:443", "0.1.2.3:443", "[::ffff:0.0.0.1]:443",
		"127.0.0.1:443", "10.0.0.1:443", "169.254.169.254:443",
		"[::1]:443", "[::ffff:127.0.0.1]:443", "downloader.disk.yandex.net:443",
		"not-a-socket-address",
	} {
		if err := academyVideoDialControl(context.Background(), "tcp", address, nil); err == nil {
			t.Fatalf("unsafe resolved destination accepted: %s", address)
		}
	}
	for _, address := range []string{"8.8.8.8:443", "[2606:4700:4700::1111]:443"} {
		if err := academyVideoDialControl(context.Background(), "tcp", address, nil); err != nil {
			t.Fatalf("public resolved destination rejected: %s: %v", address, err)
		}
	}
}

func TestAcademyVideoDialerRejectsLiveLoopbackSocket(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	dialer := net.Dialer{Timeout: time.Second, ControlContext: academyVideoDialControl}
	connection, err := dialer.DialContext(context.Background(), "tcp", listener.Addr().String())
	if connection != nil {
		connection.Close()
		t.Fatal("media gateway dialed a local service")
	}
	if err == nil || !strings.Contains(err.Error(), "non-public cloud media destination") {
		t.Fatalf("local service must be blocked before connection: %v", err)
	}
}
