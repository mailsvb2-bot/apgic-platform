package remoteconfig

import (
	"crypto/ed25519"
	"errors"
	"testing"
	"time"
)

func validPayload(now time.Time, version uint64) Payload {
	return Payload{
		Version:   version,
		IssuedAt:  now.Add(-time.Minute),
		ExpiresAt: now.Add(time.Hour),
		PolicyID:  "incident-policy-v1",
		Disabled:  []Capability{CapabilityPersonaPreview},
		ReasonCodes: map[Capability]string{
			CapabilityPersonaPreview: "INCIDENT_DISABLE",
		},
	}
}

func TestSignedConfigIsVersionedAndRetainsLastKnownSafe(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()

	first, err := Sign(validPayload(now, 1), "key-1", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	var manager Manager
	if err := manager.Apply(first, publicKey, now); err != nil {
		t.Fatal(err)
	}
	if !manager.IsDisabled(CapabilityPersonaPreview) {
		t.Fatal("kill switch was not applied")
	}

	bad := first
	bad.Payload.Version = 2
	if err := manager.Apply(bad, publicKey, now); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("tampered config must fail signature validation: %v", err)
	}
	active, ok := manager.Active()
	if !ok || active.Payload.Version != 1 {
		t.Fatalf("invalid incoming config replaced last-known-safe: %+v", active)
	}
}

func TestStaleConfigCannotRollBackActiveVersion(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	var manager Manager

	for _, version := range []uint64{1, 2} {
		envelope, err := Sign(validPayload(now, version), "key-1", privateKey)
		if err != nil {
			t.Fatal(err)
		}
		if err := manager.Apply(envelope, publicKey, now); err != nil {
			t.Fatal(err)
		}
	}

	old, err := Sign(validPayload(now, 1), "key-1", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Apply(old, publicKey, now); !errors.Is(err, ErrStaleVersion) {
		t.Fatalf("expected stale version rejection, got %v", err)
	}
}

func TestRemoteConfigCannotOwnPrivilegedBusinessTruth(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	payload := validPayload(now, 1)
	payload.Disabled = []Capability{Capability("PAYMENT_CAPTURE")}

	if _, err := Sign(payload, "key-1", privateKey); !errors.Is(err, ErrPrivilegedCapability) {
		t.Fatalf("privileged truth must not be modeled as remote capability: %v", err)
	}
}

func TestExpiredConfigIsRejected(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	payload := validPayload(now.Add(-2*time.Hour), 1)
	envelope, err := Sign(payload, "key-1", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(envelope, publicKey, now); !errors.Is(err, ErrExpiredConfig) {
		t.Fatalf("expected expired config rejection, got %v", err)
	}
}
