package runtimepostgres

import (
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
