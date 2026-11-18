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
			testAcademyRevokedHTTPTransport(t, protocol.h2)
		})
	}
}

func testAcademyRevokedHTTPTransport(t *testing.T, h2 bool) {
	revoke := make(chan struct{})
	writeResult := make(chan error, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		w.Header().Set("Content-Type", "video/mp4")
		w.WriteHeader(http.StatusOK)
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
		_, err := w.Write([]byte("forbidden-after-revoke"))
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
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
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
