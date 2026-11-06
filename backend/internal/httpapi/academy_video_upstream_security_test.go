package httpapi

import (
	"net/http"
	"testing"
)

// Upstream responses are untrusted even after the download URL has passed
// the host/IP checks. Never expose ambiguous or malformed byte metadata.
func TestAcademyVideoUpstreamRejectsAmbiguousRepresentationHeaders(t *testing.T) {
	tests := []struct {
		name    string
		request string
		status  int
		headers http.Header
	}{
		{
			name:   "duplicate content type",
			status: http.StatusOK,
			headers: http.Header{
				"Content-Type":   {"video/mp4", "text/html"},
				"Content-Length": {"100"},
			},
		},
		{
			name:   "duplicate length",
			status: http.StatusOK,
			headers: http.Header{
				"Content-Type":   {"video/mp4"},
				"Content-Length": {"100", "200"},
			},
		},
		{
			name:   "compressed full representation",
			status: http.StatusOK,
			headers: http.Header{
				"Content-Type":     {"video/mp4"},
				"Content-Length":   {"100"},
				"Content-Encoding": {"gzip"},
			},
		},
		{
			name:    "range request answered with full body",
			request: "bytes=0-9",
			status:  http.StatusOK,
			headers: http.Header{
				"Content-Type":   {"video/mp4"},
				"Content-Length": {"100"},
			},
		},
		{
			name:    "partial content with conflicting range",
			request: "bytes=0-9",
			status:  http.StatusPartialContent,
			headers: http.Header{
				"Content-Type":   {"video/mp4"},
				"Content-Range":  {"bytes 0-9/100", "bytes 10-19/100"},
				"Content-Length": {"10"},
			},
		},
		{
			name:   "html response masquerading as video",
			status: http.StatusOK,
			headers: http.Header{
				"Content-Type":   {"text/html"},
				"Content-Length": {"100"},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if academyVideoUpstreamResponseAllowed(tc.request, tc.status, tc.headers) {
				t.Fatal("untrusted upstream representation accepted")
			}
		})
	}
}

func TestAcademyVideoUnsatisfiedRangeRejectsAmbiguousMetadata(t *testing.T) {
	tests := []struct {
		name, request string
		headers       http.Header
	}{
		{"duplicate content range", "bytes=100-", http.Header{"Content-Range": {"bytes */100", "bytes */200"}}},
		{"satisfiable range", "bytes=99-", http.Header{"Content-Range": {"bytes */100"}}},
		{"invalid total", "bytes=100-", http.Header{"Content-Range": {"bytes */unknown"}}},
		{"suffix on nonempty file", "bytes=-10", http.Header{"Content-Range": {"bytes */100"}}},
		{"missing range", "", http.Header{"Content-Range": {"bytes */100"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if academyVideoUnsatisfiedRangeHeadersAllowed(tc.request, tc.headers) {
				t.Fatal("untrusted upstream 416 accepted")
			}
		})
	}
}
