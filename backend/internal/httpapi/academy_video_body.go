package httpapi

import (
	"errors"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
)

// academyVideoBodyForLearner bounds a partial response to the exact number
// of bytes promised by its validated Content-Range. A provider may use
// chunked transfer encoding (no Content-Length), but it must never be able
// to send bytes beyond the approved range to the learner.
func academyVideoBodyForLearner(status int, headers http.Header, body io.Reader) (io.Reader, error) {
	if status != http.StatusPartialContent {
		return body, nil
	}
	length, err := academyVideoPartialLength(headers.Get("Content-Range"))
	if err != nil {
		return nil, err
	}
	return io.LimitReader(body, length), nil
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
