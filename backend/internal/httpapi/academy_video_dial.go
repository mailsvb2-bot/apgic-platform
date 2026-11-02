package httpapi

import (
	"context"
	"net/netip"
	"syscall"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// Media downloads and credential-bearing Yandex API calls use the SAME
// public-IP policy. Keep these wrappers for the gateway regression tests.
func academyVideoPublicDownloadIP(ip netip.Addr) bool {
	return academy.YandexPublicIP(ip)
}

func academyVideoDialControl(ctx context.Context, network, address string, conn syscall.RawConn) error {
	return academy.YandexDialControl(ctx, network, address, conn)
}
