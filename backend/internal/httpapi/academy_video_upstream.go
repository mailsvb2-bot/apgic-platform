package httpapi

import (
	"net/http"
	"strconv"
	"strings"
)

// academyVideoUpstreamResponseAllowed rejects incorrect Range semantics and
// non-video responses before their headers or body are forwarded to learners.
// No upstream Location, Set-Cookie, authorization or server headers are copied.
func academyVideoUpstreamResponseAllowed(requestRange string, status int, headers http.Header) bool {
	if status != http.StatusOK && status != http.StatusPartialContent {
		return false
	}
	if requestRange != "" && status != http.StatusPartialContent {
		return false
	}
	// net/http Header.Get returns only the first value. Conflicting duplicate
	// upstream headers must not let a provider smuggle a different encoding,
	// media type or byte range past the gateway validation.
	if len(headers.Values("Content-Type")) != 1 || len(headers.Values("Content-Range")) > 1 ||
		len(headers.Values("Content-Length")) > 1 || len(headers.Values("Content-Encoding")) > 1 {
		return false
	}
	// Byte ranges apply to the original media representation. Never forward
	// compressed/encoded provider bytes as an unencoded HTML5 video range.
	if encoding := strings.TrimSpace(headers.Get("Content-Encoding")); encoding != "" && !strings.EqualFold(encoding, "identity") {
		return false
	}
	if status == http.StatusOK {
		// A full representation must have a known, finite length. Reject
		// chunked/unbounded 200 responses and fabricated Content-Range headers.
		if _, err := academyVideoFullLength(headers); err != nil {
			return false
		}
	}
	if status == http.StatusPartialContent &&
		!academyVideoPartialMatchesRequest(requestRange, headers.Get("Content-Range"), headers.Get("Content-Length")) {
		return false
	}
	ct := strings.ToLower(strings.TrimSpace(strings.SplitN(headers.Get("Content-Type"), ";", 2)[0]))
	return strings.HasPrefix(ct, "video/") || ct == "application/octet-stream"
}

// academyVideoUnsatisfiedRangeAllowed prevents a cloud provider from using a
// fabricated 416 response to disclose representation metadata. Only a
// syntactically valid, genuinely unsatisfiable single byte range is accepted.
func academyVideoUnsatisfiedRangeAllowed(requestRange, contentRange string) bool {
	if !academyVideoSingleRange(requestRange) || !strings.HasPrefix(contentRange, "bytes */") {
		return false
	}
	totalText := strings.TrimPrefix(contentRange, "bytes */")
	if totalText == "" || strings.Trim(totalText, "0123456789") != "" {
		return false
	}
	total, err := strconv.ParseUint(totalText, 10, 64)
	if err != nil {
		return false
	}
	spec := strings.TrimPrefix(requestRange, "bytes=")
	startText, endText, ok := strings.Cut(spec, "-")
	if !ok {
		return false
	}
	if startText == "" {
		suffix, err := strconv.ParseUint(endText, 10, 64)
		return err == nil && suffix > 0 && total == 0
	}
	start, err := strconv.ParseUint(startText, 10, 64)
	if err != nil {
		return false
	}
	if endText != "" {
		end, err := strconv.ParseUint(endText, 10, 64)
		if err != nil || end < start {
			return false
		}
	}
	return start >= total
}

// academyVideoUnsatisfiedRangeHeadersAllowed checks the complete upstream
// header set, not just Header.Get (which hides duplicate Content-Range fields).
func academyVideoUnsatisfiedRangeHeadersAllowed(requestRange string, headers http.Header) bool {
	values := headers.Values("Content-Range")
	return len(values) == 1 && academyVideoUnsatisfiedRangeAllowed(requestRange, values[0])
}
