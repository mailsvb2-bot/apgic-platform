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

func TestEmergencyKillSwitchDrill(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	var manager Manager

	baseline := validPayload(now, 1)
	baseline.Disabled = nil
	baseline.ReasonCodes = nil
	first, err := Sign(baseline, "key-1", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Apply(first, publicKey, now); err != nil {
		t.Fatal(err)
	}
	if manager.IsDisabled(CapabilityRealtimeConsultation) {
		t.Fatal("realtime must be enabled before incident kill switch")
	}

	incident := validPayload(now, 2)
	incident.Disabled = []Capability{CapabilityRealtimeConsultation}
	incident.ReasonCodes = map[Capability]string{
		CapabilityRealtimeConsultation: "INCIDENT_DISABLE_REALTIME",
	}
	second, err := Sign(incident, "key-1", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Apply(second, publicKey, now); err != nil {
		t.Fatal(err)
	}
	if !manager.IsDisabled(CapabilityRealtimeConsultation) {
		t.Fatal("incident kill switch did not disable realtime")
	}

	tampered := second
	tampered.Payload.Version = 3
	tampered.Payload.Disabled = nil
	tampered.Payload.ReasonCodes = nil
	if err := manager.Apply(tampered, publicKey, now); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("tampered recovery config must be rejected: %v", err)
	}
	active, ok := manager.Active()
	if !ok || active.Payload.Version != 2 || !manager.IsDisabled(CapabilityRealtimeConsultation) {
		t.Fatalf("last-known-safe incident state was not preserved: %+v", active)
	}

	recovery := validPayload(now, 3)
	recovery.Disabled = nil
	recovery.ReasonCodes = nil
	third, err := Sign(recovery, "key-1", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Apply(third, publicKey, now); err != nil {
		t.Fatal(err)
	}
	if manager.IsDisabled(CapabilityRealtimeConsultation) {
		t.Fatal("signed recovery config did not re-enable realtime")
	}

	privileged := validPayload(now, 4)
	privileged.Disabled = []Capability{Capability("ENTITLEMENT_GRANT")}
	if _, err := Sign(privileged, "key-1", privateKey); !errors.Is(err, ErrPrivilegedCapability) {
		t.Fatalf("kill switch drill must not gain privileged business truth: %v", err)
	}
}

func TestRemoteConfigRejectsUnknownCapabilityAndOrphanReason(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()

	unknown := validPayload(now, 1)
	unknown.Disabled = []Capability{Capability("UNKNOWN_RUNTIME_SWITCH")}
	if _, err := Sign(unknown, "key-1", privateKey); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("unknown capability must not be signed: %v", err)
	}

	orphan := validPayload(now, 2)
	orphan.Disabled = nil
	orphan.ReasonCodes = map[Capability]string{
		CapabilityPersonaPreview: "INCIDENT_DISABLE",
	}
	if _, err := Sign(orphan, "key-1", privateKey); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("reason code without disabled capability must not be signed: %v", err)
	}
}
