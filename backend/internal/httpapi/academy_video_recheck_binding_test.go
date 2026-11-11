package httpapi

import (
	"errors"
	"testing"
)

// This is a focused gateway binding regression, not the required PostgreSQL
// COMMIT -> active HTTP stream -> next Range end-to-end acceptance proof.
type academyVideoRecheckFixture struct {
	author, provider, path, token string
	sourceErr, credentialErr      error
}

func (f *academyVideoRecheckFixture) AcademyLearnerVideoSource(actor, enrollmentID, lessonID, mediaID string) (string, string, string, error) {
	return f.author, f.provider, f.path, f.sourceErr
}

func (f *academyVideoRecheckFixture) AcademyReadCloudCredential(author, provider string) (string, error) {
	return f.token, f.credentialErr
}

func TestAcademyVideoRecheckRejectsRebindingAndCredentialRotation(t *testing.T) {
	original := academyVideoRecheckFixture{
		author: "author-1", provider: "yandex_disk", path: "disk:/course/video.mp4", token: "private-token-1",
	}
	sourceAllowed := func(f *academyVideoRecheckFixture) bool {
		return academyVideoSourceStillAllowed(f, "learner-1", "enrollment-1", "lesson-1", "media-1",
			original.author, original.provider, original.path)
	}
	credentialAllowed := func(f *academyVideoRecheckFixture) bool {
		return academyVideoCloudConnectionStillAllowed(f, original.author, original.provider, original.token)
	}
	if !sourceAllowed(&original) || !credentialAllowed(&original) {
		t.Fatal("unchanged canonical binding should remain authorized")
	}
	cases := []struct {
		name   string
		change func(*academyVideoRecheckFixture)
	}{
		{"revoked enrollment", func(f *academyVideoRecheckFixture) { f.sourceErr = errors.New("revoked") }},
		{"different author", func(f *academyVideoRecheckFixture) { f.author = "author-2" }},
		{"different provider", func(f *academyVideoRecheckFixture) { f.provider = "other" }},
		{"replaced path", func(f *academyVideoRecheckFixture) { f.path = "disk:/course/replaced.mp4" }},
		{"credential rotated", func(f *academyVideoRecheckFixture) { f.token = "private-token-2" }},
		{"credential missing", func(f *academyVideoRecheckFixture) { f.token = "" }},
		{"credential store failure", func(f *academyVideoRecheckFixture) { f.credentialErr = errors.New("unavailable") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := original
			tc.change(&fixture)
			if sourceAllowed(&fixture) && credentialAllowed(&fixture) {
				t.Fatal("stale video stream remained authorized after canonical binding change")
			}
		})
	}
}

type academyVideoFingerprintRecheckFixture struct {
	resourceID, digest string
	size               int64
	err                error
}

func (f *academyVideoFingerprintRecheckFixture) AcademyLearnerVideoFingerprint(author, provider, path, mediaID string) (string, string, int64, error) {
	return f.resourceID, f.digest, f.size, f.err
}

func TestAcademyVideoFingerprintRecheckFailsClosedAfterMediaReplacement(t *testing.T) {
	const originalResource = "disk-resource-1"
	const originalDigest = "original-sha256"
	const originalSize int64 = 2048
	original := academyVideoFingerprintRecheckFixture{
		resourceID: originalResource, digest: originalDigest, size: originalSize,
	}
	allowed := func(f *academyVideoFingerprintRecheckFixture) bool {
		return academyVideoFingerprintStillAllowed(f, "author-1", "yandex_disk",
			"disk:/course/video.mp4", "media-1", originalResource, originalDigest, originalSize)
	}
	if !allowed(&original) {
		t.Fatal("unchanged verified media binding should remain authorized")
	}
	cases := []struct {
		name   string
		change func(*academyVideoFingerprintRecheckFixture)
	}{
		{"resource replaced", func(f *academyVideoFingerprintRecheckFixture) { f.resourceID = "disk-resource-2" }},
		{"digest changed", func(f *academyVideoFingerprintRecheckFixture) { f.digest = "replacement-sha256" }},
		{"size changed", func(f *academyVideoFingerprintRecheckFixture) { f.size++ }},
		{"media removed", func(f *academyVideoFingerprintRecheckFixture) { f.err = errors.New("media revoked") }},
		{"database unavailable", func(f *academyVideoFingerprintRecheckFixture) { f.err = errors.New("database unavailable") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := original
			tc.change(&fixture)
			if allowed(&fixture) {
				t.Fatal("video remained authorized after verified media binding changed")
			}
		})
	}
}
