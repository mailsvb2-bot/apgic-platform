package httpapi

import (
	"strconv"
	"strings"
)

// academyVideoPartialMatchesRequest rejects inconsistent provider 206 responses
// before their headers or bytes can reach a learner. It intentionally accepts
// only a single, fully specified Content-Range with a known representation size.
func academyVideoPartialMatchesRequest(requestRange, contentRange, contentLength string) bool {
	if !academyVideoSingleRange(requestRange) || !strings.HasPrefix(contentRange, "bytes ") {
		return false
	}
	span, totalText, ok := strings.Cut(strings.TrimPrefix(contentRange, "bytes "), "/")
	if !ok || totalText == "" || strings.Contains(totalText, "/") {
		return false
	}
	startText, endText, ok := strings.Cut(span, "-")
	if !ok || startText == "" || endText == "" {
		return false
	}
	start, errStart := strconv.ParseUint(startText, 10, 64)
	end, errEnd := strconv.ParseUint(endText, 10, 64)
	total, errTotal := strconv.ParseUint(totalText, 10, 64)
	if errStart != nil || errEnd != nil || errTotal != nil || total == 0 || start > end || end >= total {
		return false
	}
	if contentLength != "" {
		length, err := strconv.ParseUint(contentLength, 10, 64)
		if err != nil || length != end-start+1 {
			return false
		}
	}
	requested := strings.TrimPrefix(requestRange, "bytes=")
	first, last, _ := strings.Cut(requested, "-")
	if first == "" {
		suffix, err := strconv.ParseUint(last, 10, 64)
		if err != nil || suffix == 0 || end != total-1 {
			return false
		}
		expectedStart := uint64(0)
		if suffix < total {
			expectedStart = total - suffix
		}
		return start == expectedStart
	}
	requestedStart, err := strconv.ParseUint(first, 10, 64)
	if err != nil || requestedStart != start {
		return false
	}
	if last == "" {
		return end == total-1
	}
	requestedEnd, err := strconv.ParseUint(last, 10, 64)
	if err != nil || requestedEnd < requestedStart {
		return false
	}
	if requestedEnd >= total {
		requestedEnd = total - 1
	}
	return end == requestedEnd
}
