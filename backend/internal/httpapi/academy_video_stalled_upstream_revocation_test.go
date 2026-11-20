package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// A provider can stop producing bytes after the first flush. The authorization
// watcher must still cancel the request and unblock an idle upstream read.
// This is a transport regression, NOT proof of cross-process PostgreSQL
// notification or production OAuth.
func TestAcademyVideoRevocationUnblocksStalledUpstream(t *testing.T) {
	for _, tc := range []struct {
		name    string
		h2      bool
		partial bool
	}{{"http1-full", false, false}, {"http1-partial", false, true}, {"http2-full", true, false}, {"http2-partial", true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			var entitled atomic.Bool
			entitled.Store(true)
			started := make(chan struct{})
			finished := make(chan struct{})
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(finished)
				ctx, cancel := context.WithCancel(r.Context())
				defer cancel()
				upstream, writer := io.Pipe()
				defer upstream.Close()
				defer writer.Close()
				watchDone := make(chan struct{})
				go func() {
					defer close(watchDone)
					academyWatchVideoEntitlement(ctx, func() {
						cancel()
						_ = upstream.CloseWithError(context.Canceled)
						academyCancelRevokedVideoResponse(w, cancel)
					}, 5*time.Millisecond, entitled.Load)
				}()
				defer func() {
					cancel()
					<-watchDone
				}()
				w.Header().Set("Content-Type", "video/mp4")
				if tc.partial {
					w.Header().Set("Content-Range", "bytes 0-9/10")
					w.Header().Set("Content-Length", "10")
					w.WriteHeader(http.StatusPartialContent)
				} else {
					w.WriteHeader(http.StatusOK)
				}
				if _, err := w.Write([]byte("first")); err != nil {
					return
				}
				if err := http.NewResponseController(w).Flush(); err != nil {
					return
				}
				close(started)
				// No more provider bytes arrive. The read cannot finish until
				// revocation closes the upstream reader.
				buf := make([]byte, 16)
				_, _ = upstream.Read(buf)
			}))
			server.EnableHTTP2 = tc.h2
			if tc.h2 {
				server.StartTLS()
			} else {
				server.Start()
			}
			defer server.Close()
			client := server.Client()
			client.Timeout = 3 * time.Second
			resp, err := client.Get(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			wantStatus := http.StatusOK
			if tc.partial {
				wantStatus = http.StatusPartialContent
				if got := resp.Header.Get("Content-Range"); got != "bytes 0-9/10" {
					t.Fatalf("unexpected Content-Range %q", got)
				}
			}
			if resp.StatusCode != wantStatus {
				t.Fatalf("expected HTTP status %d, got %d", wantStatus, resp.StatusCode)
			}
			// Prove the HTTP/2 case negotiated HTTP/2 instead of silently
			// exercising HTTP/1.1 over TLS.
			wantProto := 1
			if tc.h2 {
				wantProto = 2
			}
			if resp.ProtoMajor != wantProto {
				t.Fatalf("expected HTTP/%d, negotiated %s", wantProto, resp.Proto)
			}
			first := make([]byte, 5)
			if _, err := io.ReadFull(resp.Body, first); err != nil {
				t.Fatal(err)
			}
			if string(first) != "first" {
				t.Fatalf("unexpected initial bytes %q", first)
			}
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("initial video response did not flush")
			}
			entitled.Store(false)
			select {
			case <-finished:
			case <-time.After(2 * time.Second):
				t.Fatal("revocation did not unblock idle upstream read")
			}
			rest, _ := io.ReadAll(resp.Body)
			if len(rest) != 0 {
				t.Fatalf("revoked stream delivered %d additional bytes", len(rest))
			}
		})
	}
}
