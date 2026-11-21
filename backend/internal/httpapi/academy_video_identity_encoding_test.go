package httpapi

import (
	"net/http"
	"testing"
)

func TestAcademyVideoIdentityEncoding(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		allow  bool
	}{
		{name: "absent is identity", allow: true},
		{name: "explicit identity", values: []string{"identity"}, allow: true},
		{name: "case insensitive identity", values: []string{" Identity "}, allow: true},
		{name: "gzip is not the verified MP4", values: []string{"gzip"}},
		{name: "br is not the verified MP4", values: []string{"br"}},
		{name: "combined codings", values: []string{"identity, gzip"}},
		{name: "ambiguous repeated headers", values: []string{"identity", "gzip"}},
		{name: "repeated identity headers are ambiguous", values: []string{"identity", "identity"}},
		{name: "comma separated identity is ambiguous", values: []string{"identity,identity"}},
		{name: "uppercase gzip rejected", values: []string{"GZIP"}},
		{name: "empty coding", values: []string{""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := make(http.Header)
			for _, value := range tt.values {
				headers.Add("Content-Encoding", value)
			}
			if got := academyVideoIdentityEncoding(headers); got != tt.allow {
				t.Fatalf("academyVideoIdentityEncoding(%q) = %v, want %v", tt.values, got, tt.allow)
			}
		})
	}
}

// A custom or future transport may transparently decompress a response and
// strip Content-Encoding. Never trust the resulting body as verified MP4 bytes.
func TestAcademyVideoOriginalRepresentationRejectsTransparentDecompression(t *testing.T) {
	for _, tt := range []struct {
		name     string
		response *http.Response
		allow    bool
	}{
		{name: "nil response fails closed"},
		{name: "plain response", response: &http.Response{Header: http.Header{}}, allow: true},
		{name: "explicit identity", response: &http.Response{Header: http.Header{"Content-Encoding": {"identity"}}}, allow: true},
		{name: "gzip header", response: &http.Response{Header: http.Header{"Content-Encoding": {"gzip"}}}},
		{name: "decoded gzip header stripped", response: &http.Response{Header: http.Header{}, Uncompressed: true}},
		{name: "decoded despite identity header", response: &http.Response{Header: http.Header{"Content-Encoding": {"identity"}}, Uncompressed: true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := academyVideoOriginalRepresentation(tt.response); got != tt.allow {
				t.Fatalf("academyVideoOriginalRepresentation() = %v, want %v", got, tt.allow)
			}
		})
	}
}

func TestAcademyVideoIdentityEncodingNoncanonicalHeaders(t *testing.T) {
	for _, tt := range []struct {
		name    string
		headers http.Header
		allow   bool
	}{
		{name: "lowercase gzip must not be invisible", headers: http.Header{"content-encoding": {"gzip"}}},
		{name: "mixed-case br must not be invisible", headers: http.Header{"CONTENT-ENCODING": {"br"}}},
		{name: "lowercase identity is accepted", headers: http.Header{"content-encoding": {"identity"}}, allow: true},
		{name: "duplicate keys of different casing rejected", headers: http.Header{"Content-Encoding": {"identity"}, "content-encoding": {"gzip"}}},
		{name: "duplicate identity keys rejected", headers: http.Header{"Content-Encoding": {"identity"}, "content-encoding": {"identity"}}},
		{name: "noncanonical empty field rejected", headers: http.Header{"content-encoding": {""}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := academyVideoIdentityEncoding(tt.headers); got != tt.allow {
				t.Fatalf("academyVideoIdentityEncoding(%v) = %v, want %v", tt.headers, got, tt.allow)
			}
		})
	}
}
