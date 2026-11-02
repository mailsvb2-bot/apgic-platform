package academy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type yandexListingTestTransport func(*http.Request) (*http.Response, error)

func (fn yandexListingTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func TestYandexDiskListFilesRejectsForeignAndMalformedEntries(t *testing.T) {
	good := YandexDiskFile{Name: "lesson.mp4", Path: "disk:/lectures/lesson.mp4", Type: "file", Size: 1024}
	tooMany := make([]YandexDiskFile, 101)
	for i := range tooMany {
		tooMany[i] = good
	}
	tests := []struct {
		name  string
		items []YandexDiskFile
		total int
		ok    bool
	}{
		{"direct child", []YandexDiskFile{good}, 1, true},
		{"empty folder", []YandexDiskFile{}, 0, true},
		{"foreign folder", []YandexDiskFile{{Name: "other.mp4", Path: "disk:/other/other.mp4", Type: "file"}}, 1, false},
		{"nested file", []YandexDiskFile{{Name: "file.mp4", Path: "disk:/lectures/nested/file.mp4", Type: "file"}}, 1, false},
		{"path traversal", []YandexDiskFile{{Name: "evil", Path: "disk:/lectures/../evil", Type: "file"}}, 1, false},
		{"malformed path", []YandexDiskFile{{Name: "file", Path: "disk:/lectures/file ", Type: "file"}}, 1, false},
		{"unknown type", []YandexDiskFile{{Name: "file", Path: "disk:/lectures/file", Type: "link"}}, 1, false},
		{"empty name", []YandexDiskFile{{Path: "disk:/lectures/file", Type: "file"}}, 1, false},
		{"negative size", []YandexDiskFile{{Name: "file", Path: "disk:/lectures/file", Type: "file", Size: -1}}, 1, false},
		{"negative total", []YandexDiskFile{good}, -1, false},
		{"oversized page", tooMany, 101, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := json.Marshal(map[string]any{
				"_embedded": map[string]any{"items": tt.items, "total": tt.total},
			})
			if err != nil {
				t.Fatal(err)
			}
			client := YandexDiskClient{Client: &http.Client{Transport: yandexListingTestTransport(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host != "cloud-api.yandex.net" || req.URL.Scheme != "https" ||
					req.URL.Query().Get("path") != "disk:/lectures" || req.Header.Get("Authorization") != "OAuth private-token" {
					t.Errorf("incorrect provider request: host=%q path=%q auth=%v",
						req.URL.Host, req.URL.Query().Get("path"), req.Header.Get("Authorization") == "OAuth private-token")
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": {"application/json"}},
					Body:       io.NopCloser(strings.NewReader(string(payload))),
				}, nil
			})}}
			items, total, err := client.ListFiles(context.Background(), "private-token", "disk:/lectures", 0, 100)
			if (err == nil) != tt.ok {
				t.Fatalf("accepted=%v, want=%v (err=%v)", err == nil, tt.ok, err)
			}
			if tt.ok && (len(items) != len(tt.items) || total != tt.total) {
				t.Fatalf("unexpected listing: %d entries, total %d", len(items), total)
			}
			if err != nil && strings.Contains(err.Error(), "private-token") {
				t.Fatal("cloud OAuth credential leaked in error")
			}
		})
	}
}
