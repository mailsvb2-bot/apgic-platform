package httpapi

import "testing"

func TestAcademyCloudBrowserQueryRejectsAmbiguousSelectors(t *testing.T) {
	tests := []struct {
		name, query, path string
		offset            int
		ok                bool
	}{
		{"default root", "", "disk:/", 0, true},
		{"selected folder", "path=disk%3A%2Flectures&offset=100", "disk:/lectures", 100, true},
		{"root page", "path=disk%3A%2F&offset=0", "disk:/", 0, true},
		{"duplicate folder", "path=disk%3A%2Fsafe&path=disk%3A%2Fother", "", 0, false},
		{"duplicate page", "offset=0&offset=100", "", 0, false},
		{"malformed escape", "path=disk%3A%2Fbad%ZZ", "", 0, false},
		{"missing path", "path=", "", 0, false},
		{"external path", "path=https%3A%2F%2Fevil.invalid", "", 0, false},
		{"leading whitespace", "path=%20disk%3A%2F", "", 0, false},
		{"trailing whitespace", "path=disk%3A%2Fbad%20", "", 0, false},
		{"unknown parameter", "path=disk%3A%2F&token=secret", "", 0, false},
		{"negative offset", "offset=-1", "", 0, false},
		{"signed offset", "offset=%2B1", "", 0, false},
		{"overflow offset", "offset=999999999999999999999", "", 0, false},
		{"excessive offset", "offset=1000001", "", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, offset, err := academyCloudBrowserQuery(tt.query)
			if (err == nil) != tt.ok {
				t.Fatalf("accepted=%v, want=%v (err=%v)", err == nil, tt.ok, err)
			}
			if tt.ok && (path != tt.path || offset != tt.offset) {
				t.Fatalf("folder=(%q,%d), want (%q,%d)", path, offset, tt.path, tt.offset)
			}
		})
	}
}
