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
			err = academyCopyVideoAuthorized(&output, response.Body, func() bool { return true }, 512, time.Second)
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
