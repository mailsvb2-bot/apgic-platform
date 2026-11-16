package runtimepostgres

import (
	"os"
	"strings"
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
	// An operator flag without an exact build identity cannot select a
	// provider acceptance record and therefore cannot open VIDEO publication.
	t.Setenv("APGIC_RELEASE_SOURCE_SHA", "")
	if academyReleaseSourceSHA() != "" {
		t.Fatal("missing release source SHA was accepted")
	}
	t.Setenv("APGIC_RELEASE_SOURCE_SHA", "not-a-commit")
	if academyReleaseSourceSHA() != "" {
		t.Fatal("malformed release source SHA was accepted")
	}
	validSHA := strings.Repeat("a", 40)
	t.Setenv("APGIC_RELEASE_SOURCE_SHA", validSHA)
	if academyReleaseSourceSHA() != validSHA {
		t.Fatal("valid exact release source SHA was rejected")
	}
}
