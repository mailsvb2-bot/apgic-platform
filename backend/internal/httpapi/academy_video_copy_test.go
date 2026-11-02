package httpapi

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestAcademyCopyVideoAuthorizedStopsBeforeRevokedChunk(t *testing.T) {
	var output bytes.Buffer
	calls := 0
	err := academyCopyVideoAuthorized(&output, strings.NewReader(strings.Repeat("v", 196608)), func() bool {
		calls++
		return calls == 1
	})
	if !errors.Is(err, errAcademyVideoRevoked) {
		t.Fatalf("expected revoked error, got %v", err)
	}
	if output.Len() != 65536 {
		t.Fatalf("revoked bytes escaped: got %d, want 65536", output.Len())
	}
}

func TestAcademyCopyVideoAuthorizedAllowsFullContent(t *testing.T) {
	input := strings.Repeat("v", 131072)
	var output bytes.Buffer
	if err := academyCopyVideoAuthorized(&output, strings.NewReader(input), func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	if output.String() != input {
		t.Fatal("authorized output was truncated")
	}
}

func TestAcademyCopyVideoAuthorizedChecksFirstChunk(t *testing.T) {
	var output bytes.Buffer
	err := academyCopyVideoAuthorized(&output, strings.NewReader("private"), func() bool { return false })
	if !errors.Is(err, errAcademyVideoRevoked) || output.Len() != 0 {
		t.Fatalf("revoked learner received bytes: error=%v bytes=%d", err, output.Len())
	}
}

type academyCredentialRevokingVideoWriter struct {
	bytes.Buffer
	source *academyVideoRevocationSource
	writes int
}

func (w *academyCredentialRevokingVideoWriter) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	w.writes++
	if w.writes == 1 {
		w.source.token = "replacement-account-secret"
	}
	return n, err
}

// A credential rotation during a live stream must be checked at the byte
// copier boundary, not only by the once-per-second background watcher.
func TestAcademyCopyVideoAuthorizedStopsOnCloudCredentialRotation(t *testing.T) {
	source := &academyVideoRevocationSource{
		author: "author", provider: "yandex_disk",
		path: "disk:/private/video.mp4", token: "original-account-secret",
	}
	writer := &academyCredentialRevokingVideoWriter{source: source}
	authorized := func() bool {
		return academyVideoSourceStillAllowed(source, "learner", "enrollment", "lesson", "media",
			"author", "yandex_disk", "disk:/private/video.mp4") &&
			academyVideoCloudConnectionStillAllowed(source, "author", "yandex_disk", "original-account-secret")
	}
	err := academyCopyVideoAuthorized(writer, strings.NewReader(strings.Repeat("v", 3*65536)), authorized)
	if !errors.Is(err, errAcademyVideoRevoked) {
		t.Fatalf("rotated credential did not stop video copy: %v", err)
	}
	if writer.Len() != 65536 || writer.writes != 1 {
		t.Fatalf("old cloud credential leaked further chunks: bytes=%d writes=%d", writer.Len(), writer.writes)
	}
}
