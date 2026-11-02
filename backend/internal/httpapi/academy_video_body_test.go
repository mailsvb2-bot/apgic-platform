package httpapi

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestAcademyVideoBodyForLearnerNeverExceedsApprovedRange(t *testing.T) {
	for _, tc := range []struct {
		name, contentRange, body, want string
		status                         int
	}{
		{"bounded chunked 206", "bytes 100-103/1000", "ABCDSECRET", "ABCD", http.StatusPartialContent},
		{"single byte", "bytes 0-0/1000", "XY", "X", http.StatusPartialContent},
		{"full 200 untouched", "", "ALL", "ALL", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader, err := academyVideoBodyForLearner(tc.status, http.Header{"Content-Range": {tc.contentRange}}, strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(reader)
			if err != nil || string(got) != tc.want {
				t.Fatalf("forwarded %q (err=%v), want %q", got, err, tc.want)
			}
		})
	}
}

func TestAcademyVideoBodyForLearnerRejectsInvalidOrUnboundedPartial(t *testing.T) {
	for _, value := range []string{
		"", "bytes */100", "bytes 0-10/*", "bytes 10-0/100",
		"bytes 0-100/100", "bytes 0-9223372036854775807/18446744073709551615",
		"bytes 0-1/2\r\nLocation: https://evil.invalid",
	} {
		if _, err := academyVideoBodyForLearner(http.StatusPartialContent, http.Header{"Content-Range": {value}}, strings.NewReader("x")); err == nil {
			t.Fatalf("invalid partial range %q accepted", value)
		}
	}
}
