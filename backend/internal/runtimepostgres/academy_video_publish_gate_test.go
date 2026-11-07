package runtimepostgres

import (
	"context"
	"os"
	"testing"
)

func TestAcademyVideoPublishDefaultsFailClosed(t *testing.T) {
	name := "APGIC_ACADEMY_VIDEO_PUBLISH_ENABLED"
	before, set := os.LookupEnv(name)
	defer func() {
		if set {
			_ = os.Setenv(name, before)
		} else {
			_ = os.Unsetenv(name)
		}
	}()
	_ = os.Unsetenv(name)
	if academyVideoPublishEnabled() {
		t.Fatal("video publication must be disabled without explicit release approval")
	}
	_ = os.Setenv(name, "TRUE")
	if academyVideoPublishEnabled() {
		t.Fatal("non-canonical enable value accepted")
	}
	_ = os.Setenv(name, "true")
	if !academyVideoPublishEnabled() {
		t.Fatal("explicit enable value rejected")
	}
}

func TestAcademyVideoPublishFlagCannotBypassPlaybackCertification(t *testing.T) {
	t.Setenv("APGIC_ACADEMY_VIDEO_PUBLISH_ENABLED", "true")
	if !academyVideoPublishEnabled() {
		t.Fatal("test requires operator flag to be enabled")
	}
	if academyVideoPlaybackCertificationAvailable() {
		t.Fatal("video publication must remain blocked until verified playback evidence exists")
	}
	// No PostgreSQL transaction should be needed for a mandatory missing
	// certification; the safety boundary must precede metadata SQL checks.
	ready, err := academyVideoPublicationReady(context.Background(), nil, "00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002")
	if err != nil || ready {
		t.Fatalf("flag bypassed mandatory proof: ready=%v err=%v", ready, err)
	}
}
