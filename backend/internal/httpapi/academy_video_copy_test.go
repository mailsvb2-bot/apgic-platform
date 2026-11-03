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

type academyFlushingVideoWriter struct {
	bytes.Buffer
	flushes int
}

func (w *academyFlushingVideoWriter) Flush() {
	w.flushes++
}

// A live upstream can pause between chunks. Without flushing each completed
// authorized write, the learner can stall while bytes sit in net/http buffers.
func TestAcademyCopyVideoAuthorizedFlushesBeforeUpstreamCompletes(t *testing.T) {
	writer := &academyFlushingVideoWriter{}
	input := strings.Repeat("v", 2*65536)
	if err := academyCopyVideoAuthorized(writer, strings.NewReader(input), func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	if writer.String() != input || writer.flushes != 2 {
		t.Fatalf("authorized stream did not flush both chunks: bytes=%d flushes=%d", writer.Len(), writer.flushes)
	}
}

type academyCredentialRevokingFlushingWriter struct {
	*academyCredentialRevokingVideoWriter
	flushes int
}

func (w *academyCredentialRevokingFlushingWriter) Flush() {
	w.flushes++
}

// Revocation during a blocked learner Write must prevent an explicit flush
// of that write, even if it had already accepted bytes into its buffer.
func TestAcademyCopyVideoAuthorizedDoesNotFlushAfterRevocationDuringWrite(t *testing.T) {
	source := &academyVideoRevocationSource{
		author: "author", provider: "yandex_disk",
		path: "disk:/private/video.mp4", token: "original-account-secret",
	}
	writer := &academyCredentialRevokingFlushingWriter{
		academyCredentialRevokingVideoWriter: &academyCredentialRevokingVideoWriter{source: source},
	}
	authorized := func() bool {
		return academyVideoSourceStillAllowed(source, "learner", "enrollment", "lesson", "media",
			"author", "yandex_disk", "disk:/private/video.mp4") &&
			academyVideoCloudConnectionStillAllowed(source, "author", "yandex_disk", "original-account-secret")
	}
	err := academyCopyVideoAuthorized(writer, strings.NewReader(strings.Repeat("v", 2*65536)), authorized)
	if !errors.Is(err, errAcademyVideoRevoked) || writer.flushes != 0 || writer.Len() != 65536 {
		t.Fatalf("revoked video was flushed: error=%v flushes=%d bytes=%d", err, writer.flushes, writer.Len())
	}
}

type academyErrorAwareRevocationWriter struct {
	bytes.Buffer
	allowed       *bool
	flushes       int
	legacyFlushes int
	flushErr      error
}

func (w *academyErrorAwareRevocationWriter) FlushError() error {
	w.flushes++
	if w.allowed != nil {
		*w.allowed = false
	}
	return w.flushErr
}

func (w *academyErrorAwareRevocationWriter) Flush() {
	w.legacyFlushes++
}

// An entitlement may be revoked while the HTTP flush is blocked. A successful
// flush does not authorize reading or sending a second cloud chunk.
func TestAcademyCopyVideoAuthorizedRechecksAfterFlush(t *testing.T) {
	allowed := true
	writer := &academyErrorAwareRevocationWriter{allowed: &allowed}
	err := academyCopyVideoAuthorized(writer, strings.NewReader(strings.Repeat("v", 2*65536)), func() bool { return allowed })
	if !errors.Is(err, errAcademyVideoRevoked) || writer.Len() != 65536 ||
		writer.flushes != 1 || writer.legacyFlushes != 0 {
		t.Fatalf("revocation during flush was not enforced: err=%v bytes=%d flushes=%d legacy=%d",
			err, writer.Len(), writer.flushes, writer.legacyFlushes)
	}
}

// net/http can report a failed flush even when Write accepted the bytes.
// Stop the upstream immediately instead of reading additional video chunks.
func TestAcademyCopyVideoAuthorizedPropagatesFlushError(t *testing.T) {
	writer := &academyErrorAwareRevocationWriter{flushErr: errors.New("downstream closed")}
	err := academyCopyVideoAuthorized(writer, strings.NewReader(strings.Repeat("v", 2*65536)), func() bool { return true })
	if err == nil || err.Error() != "downstream closed" || writer.Len() != 65536 ||
		writer.flushes != 1 || writer.legacyFlushes != 0 {
		t.Fatalf("failed flush did not stop the stream: err=%v bytes=%d flushes=%d legacy=%d",
			err, writer.Len(), writer.flushes, writer.legacyFlushes)
	}
}
