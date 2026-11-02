package httpapi

import "testing"

func TestAcademyVideoSingleRange(t *testing.T) {
	for _, value := range []string{"bytes=0-", "bytes=0-1023", "bytes=-500"} {
		if !academyVideoSingleRange(value) {
			t.Fatalf("valid range rejected: %q", value)
		}
	}
	for _, value := range []string{"bytes=", "bytes=--", "bytes=1-2,3-4", "bytes=x-y", "bytes= 1-", "items=0-2", "bytes=0-2\r\nX-Evil: 1"} {
		if academyVideoSingleRange(value) {
			t.Fatalf("invalid range accepted: %q", value)
		}
	}
}
