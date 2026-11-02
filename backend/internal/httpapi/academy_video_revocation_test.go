package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAcademyWatchVideoEntitlementCancelsStalledStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var authorized atomic.Bool
	authorized.Store(true)
	done := make(chan struct{})
	go func() {
		academyWatchVideoEntitlement(ctx, cancel, 5*time.Millisecond, authorized.Load)
		close(done)
	}()
	authorized.Store(false)
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("revoked stream did not cancel")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("revocation watcher leaked")
	}
}

type academyVideoRevocationSource struct {
	author       string
	provider     string
	path         string
	revoked      bool
	disconnected bool
}

func (s *academyVideoRevocationSource) AcademyLearnerVideoSource(actor, enrollmentID, lessonID, mediaID string) (string, string, string, error) {
	if s.revoked || s.disconnected || actor != "learner" || enrollmentID != "enrollment" || lessonID != "lesson" || mediaID != "media" {
		return "", "", "", errors.New("access denied")
	}
	return s.author, s.provider, s.path, nil
}

func (s *academyVideoRevocationSource) AcademyReadCloudCredential(actor, provider string) (string, error) {
	return "", errors.New("not used by source binding test")
}

func TestAcademyVideoSourceBindingRejectsRevocationAndRebinding(t *testing.T) {
	source := &academyVideoRevocationSource{author: "author", provider: "yandex_disk", path: "disk:/private/video.mp4"}
	allowed := func() bool {
		return academyVideoSourceStillAllowed(source, "learner", "enrollment", "lesson", "media",
			"author", "yandex_disk", "disk:/private/video.mp4")
	}
	if !allowed() {
		t.Fatal("unchanged authorized source was denied")
	}
	if academyVideoSourceStillAllowed(source, "foreign", "enrollment", "lesson", "media",
		"author", "yandex_disk", "disk:/private/video.mp4") {
		t.Fatal("foreign learner was allowed")
	}
	for _, change := range []struct {
		name   string
		mutate func()
	}{
		{"author", func() { source.author = "different-author" }},
		{"provider", func() { source.provider = "different-provider" }},
		{"path", func() { source.path = "disk:/private/other.mp4" }},
	} {
		source.author, source.provider, source.path = "author", "yandex_disk", "disk:/private/video.mp4"
		change.mutate()
		if allowed() {
			t.Fatalf("source %s changed without stopping access", change.name)
		}
	}
	source.author, source.provider, source.path = "author", "yandex_disk", "disk:/private/video.mp4"
	source.revoked = true
	if allowed() {
		t.Fatal("revoked entitlement still allowed source access")
	}
	source.revoked = false
	source.disconnected = true
	if allowed() {
		t.Fatal("disconnected cloud account still allowed source access")
	}
}

type academyBlockingVideoResponseWriter struct {
	header    http.Header
	entered   chan struct{}
	released  chan struct{}
	deadlines chan time.Time
	once      sync.Once
}

func (w *academyBlockingVideoResponseWriter) Header() http.Header { return w.header }
func (w *academyBlockingVideoResponseWriter) WriteHeader(int)     {}

func (w *academyBlockingVideoResponseWriter) Write(_ []byte) (int, error) {
	close(w.entered)
	<-w.released
	return 0, io.ErrClosedPipe
}

func (w *academyBlockingVideoResponseWriter) SetWriteDeadline(deadline time.Time) error {
	w.deadlines <- deadline
	w.once.Do(func() { close(w.released) })
	return nil
}

// The canonical access watcher must abort a blocked downstream Write as well
// as the upstream request. Without a write deadline, an already-authorized
// slow client can hold the handler open after entitlement revocation.
func TestAcademyVideoRevocationInterruptsBlockedLearnerWrite(t *testing.T) {
	writer := &academyBlockingVideoResponseWriter{
		header: make(http.Header), entered: make(chan struct{}),
		released: make(chan struct{}), deadlines: make(chan time.Time, 1),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var authorized atomic.Bool
	authorized.Store(true)
	writeDone := make(chan error, 1)
	go func() {
		_, err := writer.Write([]byte("private video"))
		writeDone <- err
	}()
	select {
	case <-writer.entered:
	case <-time.After(time.Second):
		t.Fatal("learner write did not start")
	}
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		academyWatchVideoEntitlement(ctx, func() {
			academyCancelRevokedVideoResponse(writer, cancel)
		}, 5*time.Millisecond, authorized.Load)
	}()
	authorized.Store(false)
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("upstream was not canceled")
	}
	select {
	case deadline := <-writer.deadlines:
		if deadline.After(time.Now()) {
			t.Fatal("revocation did not expire the downstream write deadline")
		}
	case <-time.After(time.Second):
		t.Fatal("downstream write deadline was not set")
	}
	select {
	case err := <-writeDone:
		if !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("blocked write did not abort: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("revoked learner write stayed blocked")
	}
	select {
	case <-watchDone:
	case <-time.After(time.Second):
		t.Fatal("entitlement watcher did not stop")
	}
}
