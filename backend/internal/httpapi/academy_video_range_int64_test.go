package httpapi

import (
	"net/http"
	"testing"
)

func TestAcademyVideoRangeRejectsUnrepresentableOffsets(t *testing.T) {
	for _, tc := range []struct {
		name    string
		value   string
		allowed bool
	}{
		{"max start", "bytes=9223372036854775807-", true},
		{"overflow start", "bytes=9223372036854775808-", false},
		{"overflow end", "bytes=0-9223372036854775808", false},
		{"overflow suffix", "bytes=-9223372036854775808", false},
		{"max suffix", "bytes=-9223372036854775807", true},
		{"inverted", "bytes=3-2", false},
		{"normal seek", "bytes=0-1023", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := make(http.Header)
			h.Set("Range", tc.value)
			_, ok := academyVideoRequestedRange(h)
			if ok != tc.allowed {
				t.Fatalf("Range acceptance = %t, want %t", ok, tc.allowed)
			}
		})
	}
}
