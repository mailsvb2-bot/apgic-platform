package httpapi

import (
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
		id string
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
