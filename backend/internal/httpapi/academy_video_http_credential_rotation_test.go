package httpapi

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Prove an OAuth reconnect interrupts bytes already flowing over real HTTP
// sockets, not merely a helper's boolean result. No provider secrets are
// sent to the browser. This is not the live Yandex/PSP acceptance gate.
func TestAcademyVideoHTTPReconnectStopsInFlightAndSeek(t *testing.T) {
	const chunk = 64 << 10
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", "131072")
		_, _ = io.WriteString(w, strings.Repeat("a", chunk))
		w.(http.Flusher).Flush()
		select {
		case <-release:
			_, _ = io.WriteString(w, strings.Repeat("b", chunk))
		case <-r.Context().Done():
		}
	}))
	defer provider.Close()
	var credential atomic.Value
	credential.Store("original")
	observed := make(chan struct{}, 1)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if credential.Load().(string) != "original" {
			http.Error(w, "access denied", http.StatusForbidden)
			return
		}
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, provider.URL, nil)
		if err != nil {
			http.Error(w, "unavailable", 502)
			return
		}
		upstream, err := provider.Client().Do(request)
		if err != nil {
			http.Error(w, "unavailable", 502)
			return
		}
		defer upstream.Body.Close()
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", "131072")
		w.WriteHeader(http.StatusOK)
		authorized := func() bool { return credential.Load().(string) == "original" }
		done := make(chan struct{})
		go func() {
			defer close(done)
			academyWatchVideoEntitlement(ctx, func() {
				academyCancelRevokedVideoResponse(w, cancel)
				select {
				case observed <- struct{}{}:
				default:
				}
			}, 5*time.Millisecond, authorized)
		}()
		defer func() { cancel(); <-done }()
		_ = academyCopyVideoAuthorized(w, upstream.Body, authorized)
	}))
	defer gateway.Close()
	client := gateway.Client()
	client.Timeout = 3 * time.Second
	response, err := client.Get(gateway.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("first status=%d", response.StatusCode)
	}
	first := make([]byte, chunk)
	if _, err = io.ReadFull(response.Body, first); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, bytes.Repeat([]byte("a"), chunk)) {
		t.Fatal("wrong authorized bytes")
	}
	credential.Store("reconnected")
	select {
	case <-observed:
	case <-time.After(2 * time.Second):
		t.Fatal("reconnect not observed")
	}
	releaseOnce.Do(func() { close(release) })
	rest, readErr := io.ReadAll(response.Body)
	if len(rest) != 0 || readErr == nil {
		t.Fatalf("reconnected client received %d bytes, error=%v", len(rest), readErr)
	}
	seek, err := http.NewRequest(http.MethodGet, gateway.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	seek.Header.Set("Range", "bytes=65536-")
	denied, err := client.Do(seek)
	if err != nil {
		t.Fatal(err)
	}
	defer denied.Body.Close()
	if denied.StatusCode != http.StatusForbidden {
		t.Fatalf("reconnected seek status=%d", denied.StatusCode)
	}
}
