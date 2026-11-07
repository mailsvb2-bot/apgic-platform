package academy

import "testing"

func TestValidateYandexDownloadURLRejectsUntrustedHosts(t *testing.T) {
	for _, u := range []string{"https://downloader.disk.yandex.net/file?a=1", "https://downloader.disk.yandex.ru/file"} {
		if !ValidateYandexDownloadURL(u) {
			t.Fatalf("provider host rejected: %s", u)
		}
	}
	for _, u := range []string{"http://downloader.disk.yandex.net/file", "https://127.0.0.1/file", "https://downloader.disk.yandex.net.evil.invalid/file", "https://downloader.disk.yandex.net:8080/file", "https://alice@downloader.disk.yandex.net/file", "https://downloader.disk.yandex.net/file#fragment"} {
		if ValidateYandexDownloadURL(u) {
			t.Fatalf("unsafe provider href accepted: %s", u)
		}
	}
}
