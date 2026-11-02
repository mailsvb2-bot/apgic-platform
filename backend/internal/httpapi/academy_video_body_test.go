package httpapi

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
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

func TestAcademyVideoBoundedBodyWithAuthorizedCopy(t *testing.T) {
	upstream := strings.NewReader("ABCDUNAUTHORIZED-TAIL")
	body, err := academyVideoBodyForLearner(http.StatusPartialContent,
		http.Header{"Content-Range": {"bytes 100-103/1000"}}, upstream)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := academyCopyVideoAuthorized(&output, body, func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	if output.String() != "ABCD" {
		t.Fatalf("unapproved bytes escaped: %q", output.String())
	}

	revoked, err := academyVideoBodyForLearner(http.StatusPartialContent,
		http.Header{"Content-Range": {"bytes 100-103/1000"}}, strings.NewReader("ABCDSECRET"))
	if err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := academyCopyVideoAuthorized(&output, revoked, func() bool { return false }); err != errAcademyVideoRevoked || output.Len() != 0 {
		t.Fatalf("revoked access leaked bytes: %q, %v", output.String(), err)
	}
}

func TestAcademyVideoChunkedPartialLengthAndTruncation(t *testing.T) {
	for _, tc := range []struct {
		name, upstream string
		truncated      bool
	}{
		{"complete", "ABCD", false},
		{"truncated", "AB", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				headers := http.Header{"Content-Range": {"bytes 100-103/1000"}}
				body, err := academyVideoBodyForLearner(http.StatusPartialContent, headers, strings.NewReader(tc.upstream))
				if err != nil {
					t.Error(err)
					return
				}
				length, err := academyVideoPartialLength(headers.Get("Content-Range"))
				if err != nil {
					t.Error(err)
					return
				}
				w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
				w.Header().Set("Content-Range", headers.Get("Content-Range"))
				w.WriteHeader(http.StatusPartialContent)
				_ = academyCopyVideoAuthorized(w, body, func() bool { return true })
			}))
			defer gateway.Close()
			response, err := gateway.Client().Get(gateway.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusPartialContent || response.ContentLength != 4 {
				t.Fatalf("unexpected partial response: status=%d length=%d", response.StatusCode, response.ContentLength)
			}
			got, readErr := io.ReadAll(response.Body)
			if string(got) != tc.upstream {
				t.Fatalf("unexpected media bytes: %q", got)
			}
			if tc.truncated {
				if !errors.Is(readErr, io.ErrUnexpectedEOF) {
					t.Fatalf("truncated video must fail visibly: %v", readErr)
				}
			} else if readErr != nil {
				t.Fatalf("complete video failed: %v", readErr)
			}
		})
	}
}
