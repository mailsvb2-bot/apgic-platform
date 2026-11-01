package httpapi

import (
	"net/http"
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
	if status == http.StatusPartialContent &&
		!academyVideoPartialMatchesRequest(requestRange, headers.Get("Content-Range"), headers.Get("Content-Length")) {
		return false
	}
	ct := strings.ToLower(strings.TrimSpace(strings.SplitN(headers.Get("Content-Type"), ";", 2)[0]))
	return strings.HasPrefix(ct, "video/") || ct == "application/octet-stream"
}
