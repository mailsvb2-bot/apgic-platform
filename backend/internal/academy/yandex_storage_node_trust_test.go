package academy

import (
	"net/url"
	"testing"
)

func TestYandexStorageNodeTrustDefaultOff(t *testing.T) {
	t.Setenv(yandexStorageHostsEnv, "")
	node, _ := url.Parse("https://s245sas.storage.yandex.net/rdisk/x?sign=secret")
	t.Setenv(yandexStorageNodeTrustEnv, "")
	if ValidateYandexStorageRedirect(node) {
		t.Fatal("unpinned node accepted")
	}
	for _, value := range []string{"true ", "TRUE", "1", "yes"} {
		t.Setenv(yandexStorageNodeTrustEnv, value)
		if ValidateYandexStorageRedirect(node) {
			t.Fatalf("accepted %q", value)
		}
	}
	t.Setenv(yandexStorageNodeTrustEnv, "true")
	if !ValidateYandexStorageRedirect(node) {
		t.Fatal("expected single-label node allowed")
	}
	for _, raw := range []string{"https://a.b.storage.yandex.net/x", "http://s245sas.storage.yandex.net/x", "https://storage.yandex.net/x", "https://s245sas.storage.yandex.net.evil.example/x"} {
		u, _ := url.Parse(raw)
		if ValidateYandexStorageRedirect(u) {
			t.Fatalf("accepted %q", raw)
		}
	}
}
