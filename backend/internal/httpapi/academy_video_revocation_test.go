package httpapi

import (
	"context"
	"errors"
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
	author   string
	provider string
	path     string
	revoked  bool
}

func (s *academyVideoRevocationSource) AcademyLearnerVideoSource(actor, enrollmentID, lessonID, mediaID string) (string, string, string, error) {
	if s.revoked || actor != "learner" || enrollmentID != "enrollment" || lessonID != "lesson" || mediaID != "media" {
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
}
