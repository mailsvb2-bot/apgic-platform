package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Exercises the actual net/http connection rather than only observing a
// cancelled context. This is a transport-level revocation proof, not a
// substitute for the PostgreSQL COMMIT -> cross-worker notification E2E.
func TestAcademyRevocationClosesLiveLearnerHTTPResponse(t *testing.T) {
	for _, protocol := range []struct {
		name string
		h2   bool
	}{{"http1", false}, {"http2", true}} {
		t.Run(protocol.name, func(t *testing.T) {
			testAcademyRevokedHTTPTransport(t, protocol.h2, false, false)
		})
	}
}

// A browser seek is a distinct 206 response; the declared range must
// leave room for additional bytes so Content-Length cannot itself reject
// the post-revocation write and create a false-positive revocation test.
// Revocation must terminate it
// with the same transport guarantees as a full 200 response.
func TestAcademyRevocationClosesLiveHTTPSeekResponse(t *testing.T) {
	for _, protocol := range []struct {
		name string
		h2   bool
	}{{"http1", false}, {"http2", true}} {
		t.Run(protocol.name, func(t *testing.T) {
			testAcademyRevokedHTTPTransport(t, protocol.h2, true, false)
		})
	}
}

// Chunked 200 responses have no declared Content-Length. Prove that the
// cancellation itself, not the declared representation length, prevents
// additional bytes on both supported HTTP transports.
func TestAcademyRevocationClosesLiveChunkedHTTPResponse(t *testing.T) {
	for _, protocol := range []struct {
		name string
		h2   bool
	}{{"http1", false}, {"http2", true}} {
		t.Run(protocol.name, func(t *testing.T) {
			testAcademyRevokedHTTPTransport(t, protocol.h2, false, true)
		})
	}
}

func testAcademyRevokedHTTPTransport(t *testing.T, h2, seek, chunked bool) {
	revoke := make(chan struct{})
	writeResult := make(chan error, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		w.Header().Set("Content-Type", "video/mp4")
		if seek {
			if r.Header.Get("Range") != "bytes=5-39" {
				writeResult <- context.Canceled
				http.Error(w, "missing seek range", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Range", "bytes 5-39/40")
			w.Header().Set("Content-Length", "35")
			w.WriteHeader(http.StatusPartialContent)
		} else {
			if !chunked {
				w.Header().Set("Content-Length", "40")
			}
			w.WriteHeader(http.StatusOK)
		}
		if _, err := w.Write([]byte("first")); err != nil {
			writeResult <- err
			return
		}
		if err := http.NewResponseController(w).Flush(); err != nil {
			writeResult <- err
			return
		}
		select {
		case <-revoke:
		case <-ctx.Done():
			writeResult <- ctx.Err()
			return
		}
		academyCancelRevokedVideoResponse(w, cancel)
		_, err := w.Write([]byte("more-bytes"))
		if err == nil {
			err = http.NewResponseController(w).Flush()
		}
		writeResult <- err
	}))
	server.EnableHTTP2 = h2
	if h2 {
		server.StartTLS()
	} else {
		server.Start()
	}
	defer server.Close()

	client := server.Client()
	client.Timeout = 3 * time.Second
	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if seek {
		req.Header.Set("Range", "bytes=5-39")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if seek {
		if resp.StatusCode != http.StatusPartialContent || resp.Header.Get("Content-Range") != "bytes 5-39/40" {
			t.Fatalf("seek response must be 206 with exact range, got %d %q", resp.StatusCode, resp.Header.Get("Content-Range"))
		}
	} else if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected full 200 response, got %d", resp.StatusCode)
	}
	// A seek must leave bytes outstanding after the first flush. Otherwise
	// the declared length, rather than revocation, could stop later writes.
	if seek && resp.ContentLength != 35 {
		t.Fatalf("seek must declare 35 bytes, got %d", resp.ContentLength)
	}
	// The chunked case is a separate proof: no declared length may
	// accidentally terminate the response before the revoke signal.
	if chunked && resp.ContentLength != -1 {
		t.Fatalf("chunked response unexpectedly declared length %d", resp.ContentLength)
	}
	if h2 && resp.ProtoMajor != 2 {
		t.Fatalf("expected HTTP/2, got %s", resp.Proto)
	}
	if !h2 && resp.ProtoMajor != 1 {
		t.Fatalf("expected HTTP/1.1, got %s", resp.Proto)
	}
	first := make([]byte, len("first"))
	if _, err := io.ReadFull(resp.Body, first); err != nil {
		t.Fatal(err)
	}
	if string(first) != "first" {
		t.Fatalf("unexpected initial video bytes: %q", first)
	}
	close(revoke)
	select {
	case err := <-writeResult:
		if err == nil {
			t.Fatal("revoked HTTP response accepted additional video bytes")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("revoked response did not terminate its live write")
	}
	remaining, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		t.Logf("revoked stream terminated with %v", readErr)
	}
	if len(remaining) != 0 {
		t.Fatalf("revoked connection delivered %d additional bytes", len(remaining))
	}
}
