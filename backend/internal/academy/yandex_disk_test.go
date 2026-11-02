package academy

import (
	"context"
	"net/http"
	"net/http/httptest"
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
