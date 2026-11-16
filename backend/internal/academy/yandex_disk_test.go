package academy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestYandexDiskClientRejectsInvalidRequests(t *testing.T) {
	client := YandexDiskClient{}
	if _, _, err := client.ListFiles(context.Background(), "secret", "https://evil.example", 0, 10); err == nil {
		t.Fatal("external path accepted")
	}
	if _, _, err := client.ListFiles(context.Background(), "secret", "disk:/", 0, 1001); err == nil {
		t.Fatal("unbounded limit accepted")
	}
	if _, err := client.TemporaryDownload(context.Background(), "secret", "https://evil.example"); err == nil {
		t.Fatal("external download accepted")
	}
}

func TestYandexDiskClientDoesNotExposeOAuthTokenInErrors(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "OAuth secret-token" {
			t.Error("missing OAuth header")
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("secret-token"))
	}))
	defer s.Close()
	client := YandexDiskClient{Client: s.Client()}
	err := client.do(context.Background(), "secret-token", s.URL, &struct{}{})
	if err == nil || err.Error() != "cloud provider response 403" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestYandexDiskClientRejectsTrailingProviderJSON(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		ok   bool
	}{
		{"single document", `{"path":"disk:/video.mp4"}`, true},
		{"whitespace after document", " {\"path\":\"disk:/video.mp4\"} \n", true},
		{"second JSON document", `{"path":"disk:/video.mp4"}{"path":"disk:/other.mp4"}`, false},
		{"trailing malformed bytes", `{"path":"disk:/video.mp4"} garbage`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "OAuth private-token" {
					t.Error("missing author OAuth credential")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			var value struct {
				Path string `json:"path"`
			}
			err := (YandexDiskClient{Client: server.Client()}).do(context.Background(), "private-token", server.URL, &value)
			if (err == nil) != tc.ok {
				t.Fatalf("provider response accepted=%v, want %v: %v", err == nil, tc.ok, err)
			}
			if err != nil && strings.Contains(err.Error(), "private-token") {
				t.Fatal("OAuth token leaked in provider error")
			}
		})
	}
}

func TestYandexDiskClientCategorizesProviderFailuresWithoutSecrets(t *testing.T) {
	cases := []struct {
		name      string
		transport http.RoundTripper
		want      error
	}{
		{
			name: "network",
			transport: yandexListingTestTransport(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("secret-token upstream connection failed")
			}),
			want: ErrYandexTransport,
		},
		{
			name: "invalid JSON",
			transport: yandexListingTestTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{malformed"))}, nil
			}),
			want: ErrYandexJSON,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := YandexDiskClient{Client: &http.Client{Transport: tc.transport}}
			var dst struct{}
			err := client.do(context.Background(), "secret-token", "https://cloud-api.yandex.net/v1/disk/resources", &dst)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got error %v, want %v", err, tc.want)
			}
			if strings.Contains(err.Error(), "secret-token") {
				t.Fatal("secret leaked in error")
			}
		})
	}
}

func TestYandexStorageRedirectRejectsFragmentsAndUntrustedHosts(t *testing.T) {
	t.Setenv(yandexStorageHostsEnv, "node.storage.yandex.net,mds.storage.yandex.net")
	cases := []struct {
		raw  string
		want bool
	}{
		{"https://node.storage.yandex.net/file?signature=opaque", true},
		{"https://mds.storage.yandex.net/file?signature=opaque", true},
		{"https://downloader.disk.yandex.net/file", true},
		{"https://storage.yandex.net/file", false},
		{"https://a.b.storage.yandex.net/file", false},
		{"https://-node.storage.yandex.net/file", false},
		{"https://node-.storage.yandex.net/file", false},
		{"https://node!.storage.yandex.net/file", false},
		{"https://" + strings.Repeat("a", 64) + ".storage.yandex.net/file", false},
		{"https://node.storage.yandex.net/file#private", false},
		{"https://node.storage.yandex.net.evil.example/file", false},
		{"https://node.storage.yandex.net:8443/file", false},
		{"http://node.storage.yandex.net/file", false},
		{"https://user@node.storage.yandex.net/file", false},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			target, err := url.Parse(tc.raw)
			if err != nil {
				t.Fatal(err)
			}
			if got := ValidateYandexStorageRedirect(target); got != tc.want {
				t.Fatalf("redirect allowed=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestYandexStorageRedirectRequiresExactConfiguredHosts(t *testing.T) {
	check := func(raw string, want bool) {
		t.Helper()
		target, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := ValidateYandexStorageRedirect(target); got != want {
			t.Fatalf("redirect %q allowed=%v, want %v", raw, got, want)
		}
	}

	t.Setenv(yandexStorageHostsEnv, "")
	check("https://node.storage.yandex.net/file", false)
	check("https://downloader.disk.yandex.net/file", true)

	t.Setenv(yandexStorageHostsEnv, "node.storage.yandex.net")
	check("https://node.storage.yandex.net/file", true)
	check("https://mds.storage.yandex.net/file", false)
	check("https://a.node.storage.yandex.net/file", false)
	check("https://node.storage.yandex.net./file", false)
	check("https://node.storage.yandex.net.evil.example/file", false)

	// One malformed or wildcard entry invalidates the whole configuration;
	// a valid first item must not mask an unsafe later item.
	t.Setenv(yandexStorageHostsEnv, "node.storage.yandex.net,*.storage.yandex.net")
	check("https://node.storage.yandex.net/file", false)
	t.Setenv(yandexStorageHostsEnv, "node.storage.yandex.net, mds.storage.yandex.net")
	check("https://node.storage.yandex.net/file", false)
}

func TestYandexMediaRedirectRejectsUntrustedHistoryAndStripsCredentials(t *testing.T) {
	t.Setenv(yandexStorageHostsEnv, "node.storage.yandex.net")
	trusted, _ := http.NewRequest(http.MethodGet, "https://downloader.disk.yandex.net/file", nil)
	untrusted, _ := http.NewRequest(http.MethodGet, "https://attacker.example/file", nil)
	next, _ := http.NewRequest(http.MethodGet, "https://node.storage.yandex.net/file", nil)
	for _, name := range []string{"Authorization", "Cookie", "Proxy-Authorization", "Referer", "Origin", "X-Api-Key", "X-Auth-Token"} {
		next.Header.Set(name, "private-value")
	}
	if err := YandexMediaCheckRedirect(next, []*http.Request{trusted, untrusted}); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("untrusted redirect history accepted: %v", err)
	}
	if err := YandexMediaCheckRedirect(next, []*http.Request{trusted}); err != nil {
		t.Fatalf("trusted redirect refused: %v", err)
	}
	for _, name := range []string{"Authorization", "Cookie", "Proxy-Authorization", "Referer", "Origin", "X-Api-Key", "X-Auth-Token"} {
		if got := next.Header.Get(name); got != "" {
			t.Fatalf("sensitive redirect header %s retained", name)
		}
	}
}
