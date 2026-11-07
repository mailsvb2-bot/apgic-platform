package academy

import "testing"

func TestTrustedYandexDownloadHostRejectsSSRF(t *testing.T) {
	accepted := []string{"downloader.disk.yandex.net", "downloader.disk.yandex.ru"}
	for _, host := range accepted {
		if !trustedYandexDownloadHost(host) {
			t.Fatalf("provider host rejected: %s", host)
		}
	}
	rejected := []string{
		"127.0.0.1", "localhost", "169.254.169.254",
		"disk.yandex.net.evil.invalid", "yandex.net",
		"evil.disk.yandex.net.evil.invalid", "internal.local",
		"other.disk.yandex.net", "other.disk.yandex.ru",
		"downloader.disk.yandex.net.evil.invalid",
	}
	for _, host := range rejected {
		if trustedYandexDownloadHost(host) {
			t.Fatalf("non-provider host accepted: %s", host)
		}
	}
}
