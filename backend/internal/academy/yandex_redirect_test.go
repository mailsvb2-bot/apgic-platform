package academy

import (
	"context"
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
