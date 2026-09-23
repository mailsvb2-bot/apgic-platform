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
