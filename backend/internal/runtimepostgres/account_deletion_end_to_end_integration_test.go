package runtimepostgres

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/privacy"
)

func deletionTestPolicy(t *testing.T, refs []string, absent bool) ([]byte, ed25519.PublicKey, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	policy := privacy.DeletionPolicy{
		Version: "approved-postgres-test-v1", Scope: "PRODUCTION",
		ProductionApproved: true, Jurisdiction: "CI-TEST",
		ApprovedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour),
		RetentionReason: "CI retention of immutable consultation and finance evidence",
		ProviderRefs:    refs, NoExternalData: absent,
		DataClasses: map[string]string{
			"PROFILE": "ERASE", "HELP_INTENT": "ANONYMIZE",
			"CONSULTATION_EVIDENCE": "RETAIN_WITH_REASON",
			"FINANCIAL_EVIDENCE":    "RETAIN_WITH_REASON",
			"CLIENT_OUTCOME":        "RETAIN_WITH_REASON", "CREDENTIAL": "ERASE",
		},
	}
	material, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := json.Marshal(privacy.SignedDeletionPolicy{
		Policy: policy, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(priv, material)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return envelope, pub, policy.Version
}

func createDeletionOwner(t *testing.T, db *Checker) string {
	t.Helper()
	id, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.ExecContext(context.Background(), "INSERT INTO identities(id) VALUES($1::uuid)", id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestR5CanonicalDeletionTransactionAndVerifiedTerminalRevocation(t *testing.T) {
	url := os.Getenv("APGIC_PERSONAL_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("PostgreSQL deletion proof URL not configured")
	}
	ctx := context.Background()
	db, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	owner := createDeletionOwner(t, db)
	foreign := createDeletionOwner(t, db)
	if _, err = db.db.ExecContext(ctx, `INSERT INTO client_personal_preferences
		(identity_id,display_name) VALUES ($1::uuid,'Секретное имя'),($2::uuid,'Чужой профиль')`, owner, foreign); err != nil {
		t.Fatal(err)
	}
	installationID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.db.ExecContext(ctx, `INSERT INTO client_installations
		(id, identity_id, platform, push_endpoint, state, updated_at)
		VALUES($1::uuid,$2::uuid,'IOS','https://push.example/device-secret','ACTIVE',now())`,
		installationID, owner); err != nil {
		t.Fatal(err)
	}
	intentID, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.db.ExecContext(ctx, `INSERT INTO help_intents
		(id,identity_id,free_text,topics,goals,context)
		VALUES($1::uuid,$2::uuid,'личная информация',ARRAY['sleep'],ARRAY['goal'],'{"private":"sensitive"}'::jsonb)`, intentID, owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.SubmitAccountDeletionRequest(ctx, owner, "WEB"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ReadAccountDeletionRequest(ctx, foreign); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("foreign identity accessed deletion request: %v", err)
	}
	if _, err = db.ReconfirmAccountDeletionRequest(ctx, owner, "session:"+strings.Repeat("c", 48), time.Now()); err != nil {
		t.Fatal(err)
	}
	signed, pub, version := deletionTestPolicy(t, nil, true)
	bad, _, _ := deletionTestPolicy(t, nil, true)
	if _, err = db.AdvanceApprovedDeletion(ctx, bad, pub, version); err == nil {
		t.Fatal("foreign policy signature authorized erasure")
	}
	var before string
	if err = db.db.QueryRowContext(ctx, "SELECT display_name FROM client_personal_preferences WHERE identity_id=$1::uuid", owner).Scan(&before); err != nil || before != "Секретное имя" {
		t.Fatal("invalid policy already changed private data")
	}
	ok, err := db.AdvanceApprovedDeletion(ctx, signed, pub, version)
	if err != nil || !ok {
		t.Fatalf("approved local transaction failed: %v moved=%v", err, ok)
	}
	var count int
	if err = db.db.QueryRowContext(ctx, "SELECT count(*) FROM client_personal_preferences WHERE identity_id=$1::uuid", owner).Scan(&count); err != nil || count != 0 {
		t.Fatalf("personal preferences not erased count=%d err=%v", count, err)
	}
	if err = db.db.QueryRowContext(ctx, "SELECT count(*) FROM client_personal_preferences WHERE identity_id=$1::uuid", foreign).Scan(&count); err != nil || count != 1 {
		t.Fatalf("foreign preferences mutated count=%d err=%v", count, err)
	}
	var pushState string
	var pushEndpoint sql.NullString
	if err = db.db.QueryRowContext(ctx, `SELECT state,push_endpoint FROM client_installations
		WHERE id=$1::uuid`, installationID).Scan(&pushState, &pushEndpoint); err != nil ||
		pushState != "REVOKED" || pushEndpoint.Valid {
		t.Fatalf("push credential survived erasure: state=%s endpoint=%v err=%v", pushState, pushEndpoint, err)
	}
	var freeText string
	if err = db.db.QueryRowContext(ctx, "SELECT free_text FROM help_intents WHERE id=$1::uuid", intentID).Scan(&freeText); err != nil || freeText == "личная информация" {
		t.Fatalf("help intent was not anonymized: %q %v", freeText, err)
	}
	if revoked, err := db.ClientIdentityDeleted(ctx, owner); err != nil || revoked {
		t.Fatal("identity revoked before external and terminal verification")
	}
	done, err := db.FinalizeVerifiedDeletion(ctx)
	if err != nil || !done {
		t.Fatalf("approved terminal transition failed: %v done=%v", err, done)
	}
	if revoked, err := db.ClientIdentityDeleted(ctx, owner); err != nil || !revoked {
		t.Fatal("terminal account retained old signed session access")
	}
	if revoked, err := db.ClientIdentityDeleted(ctx, foreign); err != nil || revoked {
		t.Fatal("another identity was revoked")
	}
	receipt, err := db.ReadAccountDeletionRequest(ctx, owner)
	if err != nil || receipt.State != "PARTIALLY_RETAINED_WITH_REASON" ||
		receipt.CompletedAt == nil || receipt.RetainedReason == "" ||
		receipt.PendingProviders != 0 {
		t.Fatalf("final receipt is misleading or incomplete: %#v %v", receipt, err)
	}
	if moved, err := db.AdvanceApprovedDeletion(ctx, signed, pub, version); err != nil || moved {
		t.Fatalf("completed request reprocessed: moved=%v err=%v", moved, err)
	}
}

func TestR5ProviderErasureRetrySignedProofAndRestartRecovery(t *testing.T) {
	url := os.Getenv("APGIC_PERSONAL_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("PostgreSQL deletion proof URL not configured")
	}
	ctx := context.Background()
	db, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	owner := createDeletionOwner(t, db)
	if _, err = db.SubmitAccountDeletionRequest(ctx, owner, "ANDROID"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ReconfirmAccountDeletionRequest(ctx, owner, "session:"+strings.Repeat("d", 48), time.Now()); err != nil {
		t.Fatal(err)
	}
	policy, policyKey, version := deletionTestPolicy(t, []string{"provider-ci"}, false)
	moved, err := db.AdvanceApprovedDeletion(ctx, policy, policyKey, version)
	if err != nil || !moved {
		t.Fatalf("failed to enqueue provider job: %v", err)
	}
	job, err := db.ClaimProviderErasure(ctx)
	if err != nil || job == nil || job.IdentityID != owner {
		t.Fatalf("claimed wrong job: %#v %v", job, err)
	}
	if err = db.FailProviderErasure(ctx, *job, "HTTP_UNAVAILABLE"); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	_, err = reopened.db.ExecContext(ctx, `UPDATE deletion_provider_dispatch
		SET next_attempt_at=now()-interval '1 second'
		WHERE request_id=$1::uuid AND provider_ref=$2`, job.RequestID, job.ProviderRef)
	if err != nil {
		t.Fatal(err)
	}
	retried, err := reopened.ClaimProviderErasure(ctx)
	if err != nil || retried == nil || retried.Attempt <= job.Attempt {
		t.Fatalf("lost leased provider delivery after process restart: %#v %v", retried, err)
	}
	if done, err := reopened.FinalizeVerifiedDeletion(ctx); err != nil || done {
		t.Fatal("terminal deletion succeeded before external erasure proof")
	}
	providerPub, providerPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	proof := privacy.ProviderErasureAttestation{
		RequestID: retried.RequestID, ProviderRef: retried.ProviderRef,
		IdentityID: owner, EvidenceRef: "provider-ci/verified/receipt",
		Erased: true, ErasedAt: time.Now().UTC(),
	}
	material, err := json.Marshal(proof)
	if err != nil {
		t.Fatal(err)
	}
	signed, err := json.Marshal(privacy.SignedProviderErasureAttestation{
		Attestation: proof, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(providerPriv, material)),
	})
	if err != nil {
		t.Fatal(err)
	}
	wrong, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err = reopened.SaveProviderErasure(ctx, *retried, signed, wrong); err == nil {
		t.Fatal("untrusted external provider signature accepted")
	}
	if err = reopened.SaveProviderErasure(ctx, *job, signed, providerPub); err == nil {
		t.Fatal("superseded provider worker committed a signed receipt")
	}
	var staleReceiptCount int
	if err = reopened.db.QueryRowContext(ctx, `SELECT count(*) FROM deletion_provider_verified_receipts
		WHERE request_id=$1::uuid AND provider_ref=$2`,
		retried.RequestID, retried.ProviderRef).Scan(&staleReceiptCount); err != nil || staleReceiptCount != 0 {
		t.Fatalf("stale provider worker persisted a receipt: count=%d err=%v", staleReceiptCount, err)
	}
	if err = reopened.SaveProviderErasure(ctx, *retried, signed, providerPub); err != nil {
		t.Fatal(err)
	}
	if done, err := reopened.FinalizeVerifiedDeletion(ctx); err != nil || !done {
		t.Fatalf("verified external erasure not finalized: done=%v err=%v", done, err)
	}
	if deleted, err := reopened.ClientIdentityDeleted(ctx, owner); err != nil || !deleted {
		t.Fatal("provider-proven terminal deletion failed to revoke client sessions")
	}
}
