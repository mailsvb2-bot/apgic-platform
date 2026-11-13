package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A real HTTP server and client exercise transport statuses, byte ranges
// and the authorized copier together, not merely isolated parsing functions.
func TestAcademyVideoGatewayHTTPTransport(t *testing.T) {
	body := strings.Repeat("v", 1024)
	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "video/mp4")
		switch r.Header.Get("Range") {
		case "":
			_, _ = io.WriteString(w, body)
		case "bytes=100-199":
			w.Header().Set("Content-Range", "bytes 100-199/1024")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = io.WriteString(w, body[100:200])
		case "bytes=9999-":
			w.Header().Set("Content-Range", "bytes */1024")
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		default:
			http.Error(w, "invalid range", http.StatusBadRequest)
		}
	}))
	defer provider.Close()
	for _, tc := range []struct {
		name, requested string
		status          int
		want            string
	}{
		{"full", "", 200, body},
		{"seek", "bytes=100-199", 206, body[100:200]},
		{"unsatisfiable", "bytes=9999-", 416, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, provider.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Range", tc.requested)
			response, err := provider.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != tc.status {
				t.Fatalf("status %d, want %d", response.StatusCode, tc.status)
			}
			if response.StatusCode == 416 {
				if response.Header.Get("Content-Range") != "bytes */1024" {
					t.Fatal("missing unsatisfied content range")
				}
				return
			}
			if !academyVideoUpstreamResponseAllowed(tc.requested, response.StatusCode, response.Header) {
				t.Fatal("valid upstream HTTP response rejected")
			}
			var output bytes.Buffer
			err = academyCopyVideoAuthorized(&output, response.Body, func() bool { return true })
			if err != nil {
				t.Fatal(err)
			}
			if output.String() != tc.want {
				t.Fatal("wrong bytes")
			}
		})
	}
	if calls.Load() != 3 {
		t.Fatalf("requests: %d", calls.Load())
	}
}

func TestAcademyVideoHTTPReadStopsAfterRevocation(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("no HTTP flusher")
		}
		_, _ = io.WriteString(w, strings.Repeat("x", 65536))
		flusher.Flush()
		<-r.Context().Done()
	}))
	defer provider.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, provider.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := provider.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var allowed atomic.Bool
	allowed.Store(true)
	watchDone := make(chan struct{})
	go func() {
		academyWatchVideoEntitlement(ctx, cancel, 5*time.Millisecond, allowed.Load)
		close(watchDone)
	}()
	allowed.Store(false)
	select {
	case <-watchDone:
	case <-time.After(2 * time.Second):
		t.Fatal("revocation watcher did not cancel HTTP stream")
	}
	_, err = io.ReadAll(response.Body)
	if err == nil || (!errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "canceled")) {
		t.Fatalf("cancelled upstream stream should fail, got %v", err)
	}
}

// Unlike a helper-only revocation test, this exercises real HTTP sockets on
// BOTH sides of the gateway. A learner that already received a full first
// chunk must not receive the second one after canonical access is revoked.
// The deliberately incomplete Content-Length must surface as a client error.
func TestAcademyVideoHTTPClientSeesRevocationMidStream(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusPartialContent} {
		for _, watcherEnabled := range []bool{false, true} {
			name := http.StatusText(status) + "/per-write-only"
			if watcherEnabled {
				name = http.StatusText(status) + "/watcher-and-per-write"
			}
			t.Run(name, func(t *testing.T) {
				testAcademyVideoHTTPClientSeesRevocationMidStream(t, status, watcherEnabled)
			})
		}
	}
}

func testAcademyVideoHTTPClientSeesRevocationMidStream(t *testing.T, status int, watcherEnabled bool) {
	const chunkSize = 64 << 10
	releaseSecond := make(chan struct{})
	var providerRequests atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerRequests.Add(1)
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", "131072")
		if status == http.StatusPartialContent {
			if r.Header.Get("Range") != "bytes=65536-196607" {
				http.Error(w, "missing expected seek range", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Range", "bytes 65536-196607/262144")
			w.WriteHeader(status)
		}
		_, _ = io.WriteString(w, strings.Repeat("a", chunkSize))
		w.(http.Flusher).Flush()
		select {
		case <-releaseSecond:
			_, _ = io.WriteString(w, strings.Repeat("b", chunkSize))
		case <-r.Context().Done():
		}
	}))
	defer provider.Close()

	var authorized atomic.Bool
	authorized.Store(true)
	revocationObserved := make(chan struct{})
	copierRevocationObserved := make(chan struct{})
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authorized.Load() {
			http.Error(w, "access denied", http.StatusForbidden)
			return
		}
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		upstreamRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, provider.URL, nil)
		if err != nil {
			http.Error(w, "unavailable", http.StatusBadGateway)
			return
		}
		if status == http.StatusPartialContent {
			upstreamRequest.Header.Set("Range", r.Header.Get("Range"))
		}
		upstream, err := provider.Client().Do(upstreamRequest)
		if err != nil {
			http.Error(w, "unavailable", http.StatusBadGateway)
			return
		}
		defer upstream.Body.Close()
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", "131072")
		if status == http.StatusPartialContent {
			w.Header().Set("Content-Range", upstream.Header.Get("Content-Range"))
		}
		w.WriteHeader(status)
		watchDone := make(chan struct{})
		go func() {
			defer close(watchDone)
			if !watcherEnabled {
				<-ctx.Done()
				return
			}
			academyWatchVideoEntitlement(ctx, func() {
				academyCancelRevokedVideoResponse(w, cancel)
				close(revocationObserved)
			}, 5*time.Millisecond, authorized.Load)
		}()
		defer func() {
			cancel()
			<-watchDone
		}()
		// A revoked stream can be stopped by the per-write entitlement check
		// before the periodic watcher runs. Both are valid fail-closed paths.
		if err := academyCopyVideoAuthorized(w, upstream.Body, authorized.Load); errors.Is(err, errAcademyVideoRevoked) {
			// Exercise the same response cancellation used by the production
			// handler, including the per-write-only path with no watcher.
			academyCancelRevokedVideoResponse(w, cancel)
			close(copierRevocationObserved)
		}
	}))
	defer gateway.Close()

	client := gateway.Client()
	client.Timeout = 3 * time.Second
	request, err := http.NewRequest(http.MethodGet, gateway.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if status == http.StatusPartialContent {
		request.Header.Set("Range", "bytes=65536-196607")
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != status {
		t.Fatalf("gateway returned %d", response.StatusCode)
	}
	if status == http.StatusPartialContent && response.Header.Get("Content-Range") != "bytes 65536-196607/262144" {
		t.Fatalf("seek lost Content-Range: %q", response.Header.Get("Content-Range"))
	}
	first := make([]byte, chunkSize)
	if _, err := io.ReadFull(response.Body, first); err != nil {
		t.Fatalf("first authorized chunk unavailable: %v", err)
	}
	if !bytes.Equal(first, bytes.Repeat([]byte("a"), chunkSize)) {
		t.Fatal("wrong first video chunk")
	}
	authorized.Store(false)
	// Unblock the upstream before waiting for revocation. Otherwise the
	// per-write check cannot run: the copier is still waiting on the provider,
	// and only the watcher path is reachable.
	close(releaseSecond)
	select {
	case <-revocationObserved:
	case <-copierRevocationObserved:
	case <-time.After(2 * time.Second):
		t.Fatal("revoked stream continued without watcher cancellation or per-write denial")
	}
	remaining, err := io.ReadAll(response.Body)
	if len(remaining) != 0 || err == nil {
		t.Fatalf("revoked learner received %d extra bytes, read error=%v", len(remaining), err)
	}
	seek, err := http.NewRequest(http.MethodGet, gateway.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	seek.Header.Set("Range", "bytes=65536-")
	seekResponse, err := client.Do(seek)
	if err != nil {
		t.Fatal(err)
	}
	defer seekResponse.Body.Close()
	if seekResponse.StatusCode != http.StatusForbidden {
		t.Fatalf("revoked seek returned %d, want 403", seekResponse.StatusCode)
	}
	// A denied seek must not expose upstream locations or credentials, even
	// when the browser inspects headers and the error body.
	if seekResponse.Header.Get("Location") != "" {
		t.Fatal("revoked seek disclosed an upstream redirect")
	}
	// Denial headers are observable to the browser too. A provider URL or
	// credential must not escape in diagnostic or custom response headers.
	for name, values := range seekResponse.Header {
		for _, value := range values {
			for _, secret := range []string{provider.URL, "Authorization", "OAuth"} {
				if strings.Contains(value, secret) {
					t.Fatalf("revoked seek header %q disclosed internal value %q", name, secret)
				}
			}
		}
	}
	deniedBody, readErr := io.ReadAll(seekResponse.Body)
	if readErr != nil {
		t.Fatalf("read revoked seek response: %v", readErr)
	}
	for _, secret := range []string{provider.URL, "Content-Range", "Authorization", "OAuth"} {
		if strings.Contains(string(deniedBody), secret) {
			t.Fatalf("revoked seek body disclosed internal value %q", secret)
		}
	}
	if got := providerRequests.Load(); got != 1 {
		t.Fatalf("revoked seek reached upstream provider: %d requests, want 1", got)
	}
}
