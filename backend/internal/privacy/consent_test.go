package privacy

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestConsentRecordIsVersionedScopedEvidence(t *testing.T) {
	now := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	record, err := NewConsentRecord(ConsentRecord{
		ID:                "11111111-1111-4111-8111-111111111111",
		SubjectID:         "22222222-2222-4222-8222-222222222222",
		Purpose:           PurposeGrowthSessionProjection,
		Scope:             GrowthConsentScope("booking-1"),
		PolicyVersion:     "growth-policy-v1",
		TextHashOrVersion: "sha256:abc",
		GrantedAt:         now,
		Source:            "WEB",
		ProofMetadata:     json.RawMessage(`{"surface":"WEB"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.Purpose != PurposeGrowthSessionProjection || record.Scope != "booking/booking-1" {
		t.Fatalf("record=%#v", record)
	}
}

func TestConsentRecordRejectsMissingEvidenceAndBackdatedRevocation(t *testing.T) {
	now := time.Now().UTC()
	base := ConsentRecord{
		ID:                "11111111-1111-4111-8111-111111111111",
		SubjectID:         "22222222-2222-4222-8222-222222222222",
		Purpose:           PurposeGrowthSessionProjection,
		Scope:             "booking/one",
		PolicyVersion:     "growth-policy-v1",
		TextHashOrVersion: "sha256:abc",
		GrantedAt:         now,
		Source:            "WEB",
		ProofMetadata:     json.RawMessage(`{"surface":"WEB"}`),
	}
	broken := base
	broken.ProofMetadata = nil
	if _, err := NewConsentRecord(broken); !errors.Is(err, ErrInvalidConsent) {
		t.Fatalf("missing proof err=%v", err)
	}
	revoked := now.Add(-time.Second)
	broken = base
	broken.RevokedAt = &revoked
	if _, err := NewConsentRecord(broken); !errors.Is(err, ErrInvalidConsent) {
		t.Fatalf("backdated revoke err=%v", err)
	}
}
