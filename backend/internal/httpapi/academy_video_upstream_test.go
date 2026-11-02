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
		{"gzip encoded range", "bytes=0-99", 206, http.Header{"Content-Type": {"video/mp4"}, "Content-Range": {"bytes 0-99/500"}, "Content-Encoding": {"gzip"}}, false},
		{"br encoded full", "", 200, http.Header{"Content-Type": {"video/mp4"}, "Content-Encoding": {"br"}}, false},
		{"identity encoded range", "bytes=0-99", 206, http.Header{"Content-Type": {"video/mp4"}, "Content-Range": {"bytes 0-99/500"}, "Content-Encoding": {"identity"}}, true},
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

func TestAcademyVideoUnsatisfiedRangeAllowed(t *testing.T) {
	for _, tc := range []struct {
		name, requested, returned string
		allowed                   bool
	}{
		{"seek past end", "bytes=9999-", "bytes */1024", true},
		{"bounded past end", "bytes=9999-10000", "bytes */1024", true},
		{"empty video", "bytes=0-", "bytes */0", true},
		{"empty video suffix", "bytes=-5", "bytes */0", true},
		{"ordinary valid seek", "bytes=100-", "bytes */1024", false},
		{"last byte seek", "bytes=1023-", "bytes */1024", false},
		{"suffix within content", "bytes=-100", "bytes */1024", false},
		{"zero suffix", "bytes=-0", "bytes */0", false},
		{"no requested range", "", "bytes */1024", false},
		{"reversed range", "bytes=2000-1000", "bytes */1024", false},
		{"provider redirect content", "bytes=9999-", "bytes 0-100/1024", false},
		{"missing total", "bytes=9999-", "bytes */", false},
		{"unknown total", "bytes=9999-", "bytes */*", false},
		{"injected header", "bytes=9999-", "bytes */1024\r\nLocation: https://evil.invalid", false},
		{"negative total", "bytes=9999-", "bytes */-1", false},
		{"numeric overflow", "bytes=9999-", "bytes */18446744073709551616", false},
		{"request overflow", "bytes=18446744073709551616-", "bytes */1024", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := academyVideoUnsatisfiedRangeAllowed(tc.requested, tc.returned); got != tc.allowed {
				t.Fatalf("unsatisfied range validation: got %v, want %v", got, tc.allowed)
			}
		})
	}
}
