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
	contentRange := headers.Get("Content-Range")
	if !strings.HasPrefix(contentRange, "bytes ") {
		return nil, errors.New("invalid partial video response")
	}
	span, totalText, ok := strings.Cut(strings.TrimPrefix(contentRange, "bytes "), "/")
	if !ok {
		return nil, errors.New("invalid partial video response")
	}
	startText, endText, ok := strings.Cut(span, "-")
	if !ok {
		return nil, errors.New("invalid partial video response")
	}
	start, startErr := strconv.ParseUint(startText, 10, 64)
	end, endErr := strconv.ParseUint(endText, 10, 64)
	total, totalErr := strconv.ParseUint(totalText, 10, 64)
	if startErr != nil || endErr != nil || totalErr != nil || total == 0 ||
		start > end || end >= total || end-start >= math.MaxInt64 {
		return nil, errors.New("invalid partial video response")
	}
	return io.LimitReader(body, int64(end-start+1)), nil
}
