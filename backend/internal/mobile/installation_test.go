package mobile

import (
	"testing"
	"time"
)

func TestPushRotationInvalidatesStaleEndpointWithoutChangingIdentity(t *testing.T) {
	now := time.Now().UTC()
	installation, err := NewInstallation(
		"install-1",
		"identity-1",
		"IOS",
		"token-old",
		now,
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := installation.RotatePushEndpoint("token-new", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if installation.IdentityID != "identity-1" {
		t.Fatal("push rotation must not duplicate or replace Identity")
	}
	if installation.CanReceivePush("token-old", 1) {
		t.Fatal("stale push endpoint must not remain eligible")
	}
	if !installation.CanReceivePush("token-new", 2) {
		t.Fatal("new endpoint generation must be eligible")
	}
}

func TestRevokedInstallationCannotReceivePush(t *testing.T) {
	now := time.Now().UTC()
	installation, err := NewInstallation("install-1", "identity-1", "ANDROID", "token-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Revoke(now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if installation.CanReceivePush("token-1", 1) {
		t.Fatal("revoked installation must not receive push")
	}
}

func TestPushRotationReplayDoesNotAdvanceGeneration(t *testing.T) {
	now := time.Now().UTC()
	installation, err := NewInstallation("install-1", "identity-1", "ios", "token-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if installation.Platform != "IOS" {
		t.Fatalf("platform=%q want IOS", installation.Platform)
	}
	if err := installation.RotatePushEndpoint("token-1", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if installation.PushGeneration != 1 {
		t.Fatalf("idempotent rotation generation=%d want=1", installation.PushGeneration)
	}
}

func TestInstallationRejectsUnknownPlatform(t *testing.T) {
	if _, err := NewInstallation("install-1", "identity-1", "WINDOWS", "token-1", time.Now().UTC()); err == nil {
		t.Fatal("unsupported platform was accepted")
	}
}

func TestRevokeReplayIsIdempotent(t *testing.T) {
	now := time.Now().UTC()
	installation, err := NewInstallation("install-1", "identity-1", "ANDROID", "token-1", now)
	if err != nil {
		t.Fatal(err)
	}
	first := now.Add(time.Minute)
	if err := installation.Revoke(first); err != nil {
		t.Fatal(err)
	}
	if err := installation.Revoke(now.Add(2 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if installation.State != InstallationRevoked || installation.PushEndpoint != "" {
		t.Fatalf("unexpected revoked installation: %#v", installation)
	}
	if !installation.UpdatedAt.Equal(first) {
		t.Fatalf("idempotent revoke rewrote evidence time: got=%s want=%s", installation.UpdatedAt, first)
	}
}
