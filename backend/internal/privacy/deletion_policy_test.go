package privacy

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func signedDeletionFixture(t *testing.T, p DeletionPolicy, private ed25519.PrivateKey) []byte {
	t.Helper()
	material, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(SignedDeletionPolicy{
		Policy: p, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(private, material)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestDeletionPolicyRequiresIndependentSignatureApprovalAndExactClasses(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	classes := make(map[string]string)
	for key, value := range deletionClasses {
		classes[key] = value
	}
	p := DeletionPolicy{
		Version: "approved-test-v1", Scope: "PRODUCTION", ProductionApproved: true,
		Jurisdiction: "TEST", ApprovedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour),
		DataClasses: classes, RetentionReason: "Approved immutable financial and consultation evidence",
		NoExternalData: true,
	}
	raw := signedDeletionFixture(t, p, priv)
	if _, err := VerifyDeletionPolicy(raw, pub, p.Version, now); err != nil {
		t.Fatalf("independently approved policy rejected: %v", err)
	}
	evil := p
	evil.ProductionApproved = false
	if _, err := VerifyDeletionPolicy(signedDeletionFixture(t, evil, priv), pub, p.Version, now); !errors.Is(err, ErrDeletionPolicyUnapproved) {
		t.Fatal("unapproved policy accepted")
	}
	if _, err := VerifyDeletionPolicy(raw, pub, "different-policy-version", now); err == nil {
		t.Fatal("wrong policy version accepted")
	}
	if _, err := VerifyDeletionPolicy(raw, pub, p.Version, now.Add(2*time.Hour)); err == nil {
		t.Fatal("expired approval accepted")
	}
	evil = p
	evil.NoExternalData = false
	if _, err := VerifyDeletionPolicy(signedDeletionFixture(t, evil, priv), pub, p.Version, now); err == nil {
		t.Fatal("missing external provider inventory accepted")
	}
	evil = p
	evil.DataClasses = map[string]string{}
	if _, err := VerifyDeletionPolicy(signedDeletionFixture(t, evil, priv), pub, p.Version, now); err == nil {
		t.Fatal("missing retained financial classification accepted")
	}
	other, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyDeletionPolicy(raw, other, p.Version, now); err == nil {
		t.Fatal("another signer's approval accepted")
	}
}

func TestProviderDeletionEvidenceCannotBeForgedOrReusedAcrossOwners(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	a := ProviderErasureAttestation{
		RequestID: "request-1", ProviderRef: "storage", IdentityID: "identity-1",
		EvidenceRef: "storage/verified/receipt-1", Erased: true, ErasedAt: now,
	}
	material, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(SignedProviderErasureAttestation{
		Attestation: a, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(priv, material)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, hash, err := VerifyProviderErasure(raw, pub, "request-1", "storage", "identity-1", now); err != nil || len(hash) != 64 {
		t.Fatalf("valid provider receipt rejected: %s %v", hash, err)
	}
	for _, identity := range []string{"identity-2", ""} {
		if _, _, err := VerifyProviderErasure(raw, pub, "request-1", "storage", identity, now); err == nil {
			t.Fatal("cross-owner erasure evidence accepted")
		}
	}
	if _, _, err := VerifyProviderErasure(raw, pub, "different-request", "storage", "identity-1", now); err == nil {
		t.Fatal("cross-request erasure evidence accepted")
	}
	a.Erased = false
	forged, err := json.Marshal(SignedProviderErasureAttestation{
		Attestation: a, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(priv, material)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := VerifyProviderErasure(forged, pub, "request-1", "storage", "identity-1", now); err == nil {
		t.Fatal("attestation tampering accepted")
	}
}
