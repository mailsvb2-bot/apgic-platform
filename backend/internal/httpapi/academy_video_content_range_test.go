package httpapi

import "testing"

func TestAcademyVideoPartialMatchesRequest(t *testing.T) {
	for _, tc := range []struct{ requested, returned, length string }{
		{"bytes=0-99", "bytes 0-99/1000", "100"},
		{"bytes=900-2000", "bytes 900-999/1000", "100"},
		{"bytes=500-", "bytes 500-999/1000", "500"},
		{"bytes=-100", "bytes 900-999/1000", "100"},
		{"bytes=-2000", "bytes 0-999/1000", "1000"},
		{"bytes=0-99", "bytes 0-99/1000", ""},
	} {
		if !academyVideoPartialMatchesRequest(tc.requested, tc.returned, tc.length) {
			t.Fatalf("valid partial response rejected: %+v", tc)
		}
	}
	for _, tc := range []struct{ requested, returned, length string }{
		{"", "bytes 0-99/1000", "100"},
		{"bytes=0-99", "bytes 100-199/1000", "100"},
		{"bytes=0-99", "bytes 0-100/1000", "101"},
		{"bytes=0-99", "bytes 0-99/*", "100"},
		{"bytes=0-99", "bytes 0-99/99", "100"},
		{"bytes=0-99", "bytes 0-99/1000", "99"},
		{"bytes=-100", "bytes 0-99/1000", "100"},
		{"bytes=100-50", "bytes 100-150/1000", "51"},
		{"bytes=0-99", "bytes 0-99/1000\r\nX-Test: yes", "100"},
		{"bytes=0-99", "bytes 0-99/1000", "not-a-number"},
	} {
		if academyVideoPartialMatchesRequest(tc.requested, tc.returned, tc.length) {
			t.Fatalf("invalid partial response accepted: %+v", tc)
		}
	}
}
