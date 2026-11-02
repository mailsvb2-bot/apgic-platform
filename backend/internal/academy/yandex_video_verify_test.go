package academy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestYandexVerifyVideoRejectsUnverifiedProviderMetadata(t *testing.T) {
	const path = "disk:/lectures/lesson.mp4"
	tests := []struct {
		name string
		body string
		ok   bool
	}{
		{"verified video", `{"name":"lesson.mp4","path":"disk:/lectures/lesson.mp4","type":"file","mime_type":"video/mp4","size":1234}`, true},
		{"wrong path", `{"name":"lesson.mp4","path":"disk:/other/lesson.mp4","type":"file","mime_type":"video/mp4","size":1234}`, false},
		{"folder", `{"name":"lesson.mp4","path":"disk:/lectures/lesson.mp4","type":"dir","mime_type":"video/mp4","size":1234}`, false},
		{"nonvideo", `{"name":"lesson.mp4","path":"disk:/lectures/lesson.mp4","type":"file","mime_type":"text/html","size":1234}`, false},
		{"empty file", `{"name":"lesson.mp4","path":"disk:/lectures/lesson.mp4","type":"file","mime_type":"video/mp4","size":0}`, false},
		{"missing fields", `{"name":"lesson.mp4"}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "OAuth private-credential" {
					t.Error("provider request missing authenticated author credential")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			file, err := (YandexDiskClient{Client: server.Client()}).verifyVideoAt(
				context.Background(), "private-credential", path, server.URL,
			)
			if tt.ok && (err != nil || file.Path != path) {
				t.Fatalf("verified video rejected: %+v, %v", file, err)
			}
			if !tt.ok && err == nil {
				t.Fatalf("unverified video accepted: %+v", file)
			}
			if err != nil && strings.Contains(err.Error(), "private-credential") {
				t.Fatal("OAuth credential leaked in verification error")
			}
		})
	}
}

func TestYandexVerifyVideoRejectsExternalPathBeforeNetwork(t *testing.T) {
	if _, err := (YandexDiskClient{}).VerifyVideo(context.Background(), "secret", "https://example.com/lesson.mp4"); err == nil {
		t.Fatal("external URL accepted as Yandex video path")
	}
}
