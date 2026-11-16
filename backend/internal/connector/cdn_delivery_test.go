package connector

import (
	"net/http"
	"testing"
)

func TestSelectCDNPlaybackRange(t *testing.T) {
	tests := []struct {
		name          string
		values        []string
		contentLength int64
		status        int
		start         int64
		end           int64
		contentRange  string
		wantRange     bool
		wantError     bool
	}{
		{name: "full representation", contentLength: 1024, status: http.StatusOK},
		{name: "closed range", values: []string{"bytes=100-199"}, contentLength: 1024,
			status: http.StatusPartialContent, start: 100, end: 199, contentRange: "bytes 100-199/1024", wantRange: true},
		{name: "open ended range", values: []string{"bytes=100-"}, contentLength: 1024,
			status: http.StatusPartialContent, start: 100, end: 1023, contentRange: "bytes 100-1023/1024", wantRange: true},
		{name: "suffix range", values: []string{"bytes=-100"}, contentLength: 1024,
			status: http.StatusPartialContent, start: 924, end: 1023, contentRange: "bytes 924-1023/1024", wantRange: true},
		{name: "suffix larger than representation", values: []string{"bytes=-2048"}, contentLength: 1024,
			status: http.StatusPartialContent, start: 0, end: 1023, contentRange: "bytes 0-1023/1024", wantRange: true},
		{name: "end clamped to representation", values: []string{"bytes=1000-2048"}, contentLength: 1024,
			status: http.StatusPartialContent, start: 1000, end: 1023, contentRange: "bytes 1000-1023/1024", wantRange: true},
		{name: "single byte at end", values: []string{"bytes=1023-1023"}, contentLength: 1024,
			status: http.StatusPartialContent, start: 1023, end: 1023, contentRange: "bytes 1023-1023/1024", wantRange: true},
		{name: "start at EOF is unsatisfied", values: []string{"bytes=1024-"}, contentLength: 1024,
			status: http.StatusRequestedRangeNotSatisfiable, contentRange: "bytes */1024"},
		{name: "start beyond EOF is unsatisfied", values: []string{"bytes=2048-4096"}, contentLength: 1024,
			status: http.StatusRequestedRangeNotSatisfiable, contentRange: "bytes */1024"},
		{name: "duplicate Range fields", values: []string{"bytes=0-9", "bytes=10-19"}, contentLength: 1024, wantError: true},
		{name: "multi range", values: []string{"bytes=0-9,20-29"}, contentLength: 1024, wantError: true},
		{name: "wrong unit", values: []string{"items=0-9"}, contentLength: 1024, wantError: true},
		{name: "empty range", values: []string{"bytes="}, contentLength: 1024, wantError: true},
		{name: "reversed range", values: []string{"bytes=20-10"}, contentLength: 1024, wantError: true},
		{name: "zero suffix", values: []string{"bytes=-0"}, contentLength: 1024, wantError: true},
		{name: "signed start", values: []string{"bytes=+1-2"}, contentLength: 1024, wantError: true},
		{name: "whitespace", values: []string{"bytes=1 -2"}, contentLength: 1024, wantError: true},
		{name: "header injection", values: []string{"bytes=0-9\r\nX-Evil: 1"}, contentLength: 1024, wantError: true},
		{name: "start overflow", values: []string{"bytes=9223372036854775808-"}, contentLength: 1024, wantError: true},
		{name: "end overflow", values: []string{"bytes=0-9223372036854775808"}, contentLength: 1024, wantError: true},
		{name: "suffix overflow", values: []string{"bytes=-9223372036854775808"}, contentLength: 1024, wantError: true},
		{name: "unknown extra separator", values: []string{"bytes=1-2-3"}, contentLength: 1024, wantError: true},
		{name: "zero content length", contentLength: 0, wantError: true},
		{name: "negative content length", contentLength: -1, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := SelectCDNPlaybackRange(test.values, test.contentLength)
			if test.wantError {
				if err == nil {
					t.Fatalf("invalid range selection accepted: %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("valid range selection rejected: %v", err)
			}
			if got.StatusCode != test.status || got.ContentRange != test.contentRange {
				t.Fatalf("selection=%+v, want status=%d content-range=%q",
					got, test.status, test.contentRange)
			}
			if test.wantRange {
				if got.ByteRange == nil || got.ByteRange.Start != test.start || got.ByteRange.End != test.end {
					t.Fatalf("byte range=%+v, want %d-%d", got.ByteRange, test.start, test.end)
				}
			} else if got.ByteRange != nil {
				t.Fatalf("unexpected provider range for status %d: %+v", test.status, got.ByteRange)
			}
		})
	}
}
