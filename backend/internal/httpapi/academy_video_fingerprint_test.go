package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

func TestAcademyVideoFingerprintRejectsReplacedCloudFile(t *testing.T) {
	const digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	file := academy.YandexDiskFile{
		Path: "disk:/lesson.mp4", Type: "file", MimeType: "video/mp4",
		ResourceID: "resource-original", SHA256: digest, Size: 1024,
	}
	if !academyVideoFingerprintMatches(file, "resource-original", digest, 1024) {
		t.Fatal("verified file fingerprint rejected")
	}
	for _, tc := range []struct {
		name string
		file academy.YandexDiskFile
		id   string
		hash string
		size int64
	}{
		{"different resource", file, "resource-other", digest, 1024},
		{"different digest", file, "resource-original", strings.Repeat("b", 64), 1024},
		{"different size", file, "resource-original", digest, 2048},
		{"missing identity", file, "", digest, 1024},
		{"missing hash", file, "resource-original", "", 1024},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if academyVideoFingerprintMatches(tc.file, tc.id, tc.hash, tc.size) {
				t.Fatal("unverified cloud file was accepted for playback")
			}
		})
	}
}

func TestAcademyVideoRepresentationSizeMatchesVerifiedFile(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		header http.Header
		want   bool
	}{
		{"full", 200, http.Header{"Content-Length": {"1024"}}, true},
		{"wrong full size", 200, http.Header{"Content-Length": {"2048"}}, false},
		{"seek", 206, http.Header{"Content-Range": {"bytes 100-199/1024"}}, true},
		{"seek from replaced file", 206, http.Header{"Content-Range": {"bytes 100-199/2048"}}, false},
		{"out of range", 416, http.Header{"Content-Range": {"bytes */1024"}}, true},
		{"stale 416", 416, http.Header{"Content-Range": {"bytes */2048"}}, false},
		{"malformed total", 206, http.Header{"Content-Range": {"bytes 0-10/*"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := academyVideoRepresentationSizeMatches(tc.status, tc.header, 1024); got != tc.want {
				t.Fatalf("verified size match=%v, want %v", got, tc.want)
			}
		})
	}
}
