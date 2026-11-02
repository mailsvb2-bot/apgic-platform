package httpapi

import (
	"net/http"
	"testing"
)

func TestAcademyVideoSingleRange(t *testing.T) {
	for _, value := range []string{"bytes=0-", "bytes=0-1023", "bytes=-500"} {
		if !academyVideoSingleRange(value) {
			t.Fatalf("valid range rejected: %q", value)
		}
	}
	for _, value := range []string{"bytes=", "bytes=--", "bytes=1-2,3-4", "bytes=x-y", "bytes= 1-", "items=0-2", "bytes=0-2\r\nX-Evil: 1", "bytes=-0", "bytes=10-9", "bytes=18446744073709551616-", "bytes=0-18446744073709551616"} {
		if academyVideoSingleRange(value) {
			t.Fatalf("invalid range accepted: %q", value)
		}
	}
}

func TestAcademyVideoRequestedRangeRejectsDuplicateAndMalformedHeaders(t *testing.T) {
	for _, tc := range []struct {
		name    string
		headers http.Header
		want    string
		ok      bool
	}{
		{"absent", http.Header{}, "", true},
		{"valid seek", http.Header{"Range": {"bytes=100-200"}}, "bytes=100-200", true},
		{"valid suffix", http.Header{"Range": {"bytes=-100"}}, "bytes=-100", true},
		{"duplicate conflicting", http.Header{"Range": {"bytes=0-99", "bytes=100-199"}}, "", false},
		{"duplicate identical", http.Header{"Range": {"bytes=0-99", "bytes=0-99"}}, "", false},
		{"combined ranges", http.Header{"Range": {"bytes=0-99,100-199"}}, "", false},
		{"zero suffix", http.Header{"Range": {"bytes=-0"}}, "", false},
		{"reversed", http.Header{"Range": {"bytes=99-1"}}, "", false},
		{"overflow", http.Header{"Range": {"bytes=18446744073709551616-"}}, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value, ok := academyVideoRequestedRange(tc.headers)
			if value != tc.want || ok != tc.ok {
				t.Fatalf("range=(%q,%v), want (%q,%v)", value, ok, tc.want, tc.ok)
			}
		})
	}
}
