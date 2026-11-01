package academy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestYandexDiskClientRejectsOAuthRedirectEvenWithPermissiveClient(t *testing.T) {
	var redirected atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			if r.Header.Get("Authorization") != "OAuth sensitive-access-token" {
				t.Error("provider request missing OAuth token")
			}
			http.Redirect(w, r, "/target", http.StatusFound)
			return
		}
		redirected.Add(1)
		if r.Header.Get("Authorization") != "" {
			t.Error("OAuth token forwarded to redirect target")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client := server.Client()
	// Even a caller-provided client configured to follow redirects must not override the guard.
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return nil }
	disk := YandexDiskClient{Client: client}
	err := disk.do(context.Background(), "sensitive-access-token", server.URL+"/redirect", &struct{}{})
	if err == nil || !strings.Contains(err.Error(), "cloud provider response 302") {
		t.Fatalf("redirect must fail closed: %v", err)
	}
	if strings.Contains(err.Error(), "sensitive-access-token") {
		t.Fatal("OAuth token leaked into error")
	}
	if redirected.Load() != 0 {
		t.Fatal("redirect target was contacted")
	}
}

type yandexStaticDownloadTransport struct {
	href string
}

func (transport yandexStaticDownloadTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body := fmt.Sprintf(`{"href":%q,"method":"GET"}`, transport.href)
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestYandexDiskTemporaryDownloadRejectsUnapprovedTargets(t *testing.T) {
	cases := []struct {
		name string
		href string
		allow bool
	}{
		{"known provider", "https://downloader.disk.yandex.net/get?token=opaque", true},
		{"known provider explicit TLS", "https://downloader.disk.yandex.ru:443/get", true},
		{"unexpected port", "https://downloader.disk.yandex.net:4444/get", false},
		{"unapproved subdomain", "https://other.disk.yandex.net/get", false},
		{"host suffix spoof", "https://downloader.disk.yandex.net.evil.invalid/get", false},
		{"credential in URL", "https://user:pass@downloader.disk.yandex.net/get", false},
		{"plain HTTP", "http://downloader.disk.yandex.net/get", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := YandexDiskClient{Client: &http.Client{
				Transport: yandexStaticDownloadTransport{href: tc.href},
			}}
			href, err := client.TemporaryDownload(context.Background(), "secret", "disk:/video.mp4")
			if tc.allow && (err != nil || href != tc.href) {
				t.Fatalf("approved download rejected: %v", err)
			}
			if !tc.allow && (err == nil || href != "") {
				t.Fatalf("unsafe download accepted: %q", href)
			}
		})
	}
}
