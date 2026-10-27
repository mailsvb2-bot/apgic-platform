package privacy

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"testing"
	"time"
)

func TestDeletionExecutionRequiresRealSignedLegalApproval(t *testing.T) {
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	p := ApprovedDeletionPolicy{
		Version: "legal-approved-1", Jurisdiction: "TEST", ApprovedBy: "independent-legal-review",
		ApprovedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour),
		ProductionApproved: true,
		DataClasses: map[string]string{
			"PERSONAL_PREFERENCES":      "ERASE",
			"HELP_INTENT_PERSONAL_DATA": "ANONYMIZE",
			"PUSH_ENDPOINTS":            "ERASE",
			"FINANCIAL_EVIDENCE":        "RETAIN_WITH_REASON",
			"CONSULTATION_EVIDENCE":     "RETAIN_WITH_REASON",
			"CONSENT_EVIDENCE":          "RETAIN_WITH_REASON",
		},
		FinancialRetentionReason:    "required fiscal retention",
		ConsultationRetentionReason: "required consultation record retention",
		ConsentRetentionReason:      "required consent evidence retention",
		ProviderRefs:                []string{},
		ProviderInventoryEvidence:   "inventory-approved-no-external-provider-accounts",
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	signed := ed25519.Sign(private, raw)
	approved, err := VerifyDeletionPolicy(raw, signed, pub, now)
	if err != nil {
		t.Fatalf("valid signed policy rejected: %v", err)
	}
	if len(approved.Signature()) != 64 || len(approved.Digest()) != 64 {
		t.Fatal("missing immutable signature evidence")
	}
	changed := approved.Policy()
	changed.DataClasses["FINANCIAL_EVIDENCE"] = "ERASE"
	if approved.Policy().DataClasses["FINANCIAL_EVIDENCE"] != "RETAIN_WITH_REASON" {
		t.Fatal("caller changed signed policy map after verification")
	}
	raw[0] ^= 1
	if _, err := VerifyDeletionPolicy(raw, signed, pub, now); err == nil {
		t.Fatal("tampered policy accepted")
	}
	p.ProductionApproved = false
	ciRaw, _ := json.Marshal(p)
	if _, err := VerifyDeletionPolicy(ciRaw, ed25519.Sign(private, ciRaw), pub, now); err == nil {
		t.Fatal("CI-only/unapproved policy could initiate irreversible erasure")
	}
}

func TestExternalErasureRequiresBoundProviderSignature(t *testing.T) {
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	statement := ProviderErasureStatement{
		RequestID:   "11111111-1111-4111-8111-111111111111",
		ProviderRef: "object-storage",
		EvidenceRef: "provider-receipt-23839",
		State:       "ERASED",
		ErasedAt:    now,
	}
	raw, _ := json.Marshal(statement)
	signature := ed25519.Sign(private, raw)
	verified, err := VerifyProviderErasureReceipt(raw, signature, pub, "object-storage", now)
	if err != nil || verified.Statement().RequestID != statement.RequestID {
		t.Fatalf("valid provider evidence rejected: %v", err)
	}
	if _, err = VerifyProviderErasureReceipt(raw, signature, pub, "other-provider", now); err == nil {
		t.Fatal("provider substitution accepted")
	}
	statement.EvidenceRef = "synthetic:pretend"
	synthetic, _ := json.Marshal(statement)
	if _, err = VerifyProviderErasureReceipt(synthetic, ed25519.Sign(private, synthetic), pub, "object-storage", now); err == nil {
		t.Fatal("synthetic provider erasure accepted")
	}
}
