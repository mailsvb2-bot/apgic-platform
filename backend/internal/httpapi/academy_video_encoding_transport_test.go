package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Exercise a real HTTP transport, not just a fabricated http.Response. A
// provider can ignore Accept-Encoding and return gzip even for a Range seek.
// The gateway must reject that body for every supported response status.
func TestAcademyVideoUpstreamEncodingOverHTTP(t *testing.T) {
	for _, status := range []int{
		http.StatusOK,
		http.StatusPartialContent,
		http.StatusRequestedRangeNotSatisfiable,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("Accept-Encoding"); got != "identity" {
					t.Errorf("upstream encoding request = %q, want identity", got)
				}
				w.Header().Set("Content-Encoding", "gzip")
				w.Header().Set("Content-Type", "video/mp4")
				w.WriteHeader(status)
				_, _ = w.Write([]byte("encoded-provider-body"))
			}))
			defer origin.Close()

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, origin.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Accept-Encoding", "identity")
			response, err := academyVideoUpstreamClient().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != status {
				t.Fatalf("status = %d, want %d", response.StatusCode, status)
			}
			if academyVideoOriginalRepresentation(response) {
				t.Fatalf("encoded provider response %d passed original-representation gate", status)
			}
		})
	}
}
