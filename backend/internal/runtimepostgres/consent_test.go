package runtimepostgres

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/privacy"
)

func TestConsentLedgerGrantRevokeAndHistory(t *testing.T) {
	databaseURL := os.Getenv("APGIC_CONSENT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("consent integration database not configured")
	}
	store, err := Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	consentID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	subjectID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	input := privacy.ConsentRecord{
		ID:                consentID,
		SubjectID:         subjectID,
		Purpose:           privacy.PurposeGrowthSessionProjection,
		Scope:             "booking/test-booking",
		PolicyVersion:     "growth-policy-v1",
		TextHashOrVersion: "sha256:test-consent-text",
		GrantedAt:         now,
		Source:            "CI",
		ProofMetadata:     json.RawMessage(`{"surface":"CI","action":"explicit_grant"}`),
	}
	granted, idempotent, err := store.RecordConsent(input)
	if err != nil || idempotent {
		t.Fatalf("grant=%#v idempotent=%v err=%v", granted, idempotent, err)
	}
	active, found, err := store.ActiveConsent(subjectID, input.Purpose, input.Scope, now.Add(time.Second))
	if err != nil || !found || active.ID != consentID {
		t.Fatalf("active=%#v found=%v err=%v", active, found, err)
	}

	replayed, idempotent, err := store.RecordConsent(input)
	if err != nil || !idempotent || replayed.ID != consentID {
		t.Fatalf("replay=%#v idempotent=%v err=%v", replayed, idempotent, err)
	}

	revokedAt := now.Add(2 * time.Second)
	revoked, idempotent, err := store.RevokeConsent(consentID, subjectID, revokedAt)
	if err != nil || idempotent || revoked.RevokedAt == nil {
		t.Fatalf("revoke=%#v idempotent=%v err=%v", revoked, idempotent, err)
	}
	if _, found, err := store.ActiveConsent(subjectID, input.Purpose, input.Scope, revokedAt.Add(time.Second)); err != nil || found {
		t.Fatalf("revoked consent remained active found=%v err=%v", found, err)
	}
	replayedRevoke, idempotent, err := store.RevokeConsent(consentID, subjectID, revokedAt.Add(time.Second))
	if err != nil || !idempotent || replayedRevoke.RevokedAt == nil || !replayedRevoke.RevokedAt.Equal(revokedAt) {
		t.Fatalf("revoke replay=%#v idempotent=%v err=%v", replayedRevoke, idempotent, err)
	}

	var history int
	if err := store.db.QueryRow(
		"SELECT count(*) FROM consent_records WHERE consent_id=$1::uuid AND subject_id=$2::uuid",
		consentID, subjectID,
	).Scan(&history); err != nil {
		t.Fatal(err)
	}
	if history != 1 {
		t.Fatalf("consent history rows=%d want 1", history)
	}
}
