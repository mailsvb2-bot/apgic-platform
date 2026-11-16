package httpapi

import (
	"net/http"
	"testing"
)

// The delivery receipt is only for successfully completed HTTP transfers.
// These ranges are not proof of human watching or completed study.
func TestAcademyDeliveredVideoInterval(t *testing.T) {
	for _, tc := range []struct {
		name          string
		status        int
		rangeHeader   string
		contentLength string
		size          int64
		wantStart     int64
		wantEnd       int64
		ok            bool
	}{
		{"full", 200, "", "1000", 1000, 0, 1000, true},
		{"partial", 206, "bytes 100-199/1000", "100", 1000, 100, 200, true},
		{"last-byte", 206, "bytes 999-999/1000", "1", 1000, 999, 1000, true},
		{"no-bounds", 200, "", "", 1000, 0, 0, false},
		{"wrong-size", 200, "", "999", 1000, 0, 0, false},
		{"wrong-total", 206, "bytes 0-99/999", "100", 1000, 0, 0, false},
		{"too-long", 206, "bytes 0-1000/1000", "1001", 1000, 0, 0, false},
		{"reversed", 206, "bytes 300-200/1000", "0", 1000, 0, 0, false},
		{"unsatisfied", 416, "bytes */1000", "", 1000, 0, 0, false},
		{"invalid", 200, "", "1000", 0, 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			headers := http.Header{}
			if tc.rangeHeader != "" {
				headers.Set("Content-Range", tc.rangeHeader)
			}
			if tc.contentLength != "" {
				headers.Set("Content-Length", tc.contentLength)
			}
			start, end, ok := academyDeliveredVideoInterval(tc.status, headers, tc.size)
			if start != tc.wantStart || end != tc.wantEnd || ok != tc.ok {
				t.Fatalf("interval=(%d,%d,%t) want=(%d,%d,%t)",
					start, end, ok, tc.wantStart, tc.wantEnd, tc.ok)
			}
		})
	}
}
