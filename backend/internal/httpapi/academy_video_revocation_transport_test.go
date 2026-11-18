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
	revoke := make(chan struct{})
	writeResult := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		writeResult <- err
	}))
	defer server.Close()

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
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
	remaining, _ := io.ReadAll(resp.Body)
	if len(remaining) != 0 {
		t.Fatalf("revoked connection delivered %d additional bytes", len(remaining))
	}
}
