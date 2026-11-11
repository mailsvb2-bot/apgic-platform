package privacy

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestProviderErasureRejectsSyntheticAndAmbiguousSignedEvidence(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	proof := ProviderErasureAttestation{
		RequestID: "request-1", ProviderRef: "storage", IdentityID: "identity-1",
		EvidenceRef: "storage/verified/receipt-1", Erased: true, ErasedAt: now,
	}
	sign := func(a ProviderErasureAttestation) []byte {
		t.Helper()
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
		return raw
	}
	valid := sign(proof)
	if _, _, err := VerifyProviderErasure(valid, pub, proof.RequestID, proof.ProviderRef, proof.IdentityID, now); err != nil {
		t.Fatalf("genuine signed attestation rejected: %v", err)
	}
	for _, evidence := range []string{"provider-erasure:identity-1", "synthetic:receipt", "   Synthetic:receipt", " PROVIDER-ERASURE:identity-1", "    "} {
		synthetic := proof
		synthetic.EvidenceRef = evidence
		if _, _, err := VerifyProviderErasure(sign(synthetic), pub, proof.RequestID, proof.ProviderRef, proof.IdentityID, now); err == nil {
			t.Fatalf("placeholder evidence %q accepted with valid signature", evidence)
		}
	}
	for name, altered := range map[string][]byte{
		"unknown_envelope": []byte(strings.Replace(string(valid), "{", "{"+`"unknown":true,`, 1)),
		"unknown_nested":   []byte(strings.Replace(string(valid), `"erased":true`, `"erased":true,"unapproved":true`, 1)),
		"concatenated":     append(append([]byte(nil), valid...), []byte("{}")...),
	} {
		if _, _, err := VerifyProviderErasure(altered, pub, proof.RequestID, proof.ProviderRef, proof.IdentityID, now); err == nil {
			t.Fatalf("%s accepted despite ambiguous provider evidence", name)
		}
	}
}
