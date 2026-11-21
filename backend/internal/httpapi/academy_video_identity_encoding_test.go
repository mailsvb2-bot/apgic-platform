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
