package httpapi

import (
	"errors"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
)

// academyVideoBodyForLearner bounds both full and partial responses to the
// exact declared representation length. A malicious or misconfigured provider
// cannot send unlimited bytes on a chunked 200 or append bytes to a 206.
func academyVideoBodyForLearner(status int, headers http.Header, body io.Reader) (io.Reader, error) {
	var length int64
	var err error
	switch status {
	case http.StatusOK:
		length, err = academyVideoFullLength(headers)
	case http.StatusPartialContent:
		length, err = academyVideoPartialLength(headers.Get("Content-Range"))
	default:
		return nil, errors.New("invalid video response status")
	}
	if err != nil {
		return nil, err
	}
	return io.LimitReader(body, length), nil
}

// academyVideoFullLength rejects unbounded or ambiguous HTTP 200 responses.
// The advertised length also lets the browser detect a truncated transfer.
func academyVideoFullLength(headers http.Header) (int64, error) {
	if headers.Get("Content-Range") != "" || len(headers.Values("Content-Length")) != 1 {
		return 0, errors.New("invalid full video response")
	}
	value := headers.Get("Content-Length")
	if value == "" || strings.Trim(value, "0123456789") != "" {
		return 0, errors.New("invalid full video length")
	}
	length, err := strconv.ParseInt(value, 10, 64)
	if err != nil || length <= 0 {
		return 0, errors.New("invalid full video length")
	}
	return length, nil
}

// academyVideoPartialLength derives the exact representation length from
// the validated range, including when the provider uses chunked transfer.
// The gateway must advertise this length to browsers: otherwise a truncated
// chunked 206 can appear to finish successfully instead of failing closed.
func academyVideoPartialLength(contentRange string) (int64, error) {
	if !strings.HasPrefix(contentRange, "bytes ") {
		return 0, errors.New("invalid partial video response")
	}
	span, totalText, ok := strings.Cut(strings.TrimPrefix(contentRange, "bytes "), "/")
	if !ok {
		return 0, errors.New("invalid partial video response")
	}
	startText, endText, ok := strings.Cut(span, "-")
	if !ok {
		return 0, errors.New("invalid partial video response")
	}
	start, startErr := strconv.ParseUint(startText, 10, 64)
	end, endErr := strconv.ParseUint(endText, 10, 64)
	total, totalErr := strconv.ParseUint(totalText, 10, 64)
	if startErr != nil || endErr != nil || totalErr != nil || total == 0 ||
		start > end || end >= total || end-start >= math.MaxInt64 {
		return 0, errors.New("invalid partial video response")
	}
	return int64(end - start + 1), nil
}
