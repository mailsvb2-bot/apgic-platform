package httpapi

import (
	"net/http"
	"testing"
)

func TestAcademyVideoUpstreamResponseAllowed(t *testing.T) {
	for _, tc := range []struct {
		name, requestRange string
		status             int
		headers            http.Header
		allowed            bool
	}{
		{"full video", "", 200, http.Header{"Content-Type": {"video/mp4"}}, true},
		{"full octet stream", "", 200, http.Header{"Content-Type": {"application/octet-stream"}}, true},
		{"partial video", "bytes=0-99", 206, http.Header{"Content-Type": {"video/mp4"}, "Content-Range": {"bytes 0-99/500"}, "Content-Length": {"100"}}, true},
		{"ignored range", "bytes=0-99", 200, http.Header{"Content-Type": {"video/mp4"}}, false},
		{"unexpected partial", "", 206, http.Header{"Content-Type": {"video/mp4"}, "Content-Range": {"bytes 0-99/500"}}, false},
		{"bad range", "bytes=0-99", 206, http.Header{"Content-Type": {"video/mp4"}, "Content-Range": {"bytes 200-299/500"}}, false},
		{"html error", "", 200, http.Header{"Content-Type": {"text/html"}}, false},
		{"redirect", "", 302, http.Header{"Content-Type": {"video/mp4"}, "Location": {"https://example.com"}}, false},
		{"provider error", "", 401, http.Header{"Content-Type": {"video/mp4"}}, false},
		{"empty content type", "", 200, http.Header{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := academyVideoUpstreamResponseAllowed(tc.requestRange, tc.status, tc.headers); got != tc.allowed {
				t.Fatalf("allowed=%v, want %v", got, tc.allowed)
			}
		})
	}
}
