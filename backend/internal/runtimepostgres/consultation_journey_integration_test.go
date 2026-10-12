package runtimepostgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/connector"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/communication"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/persistentid"
)

// The isolated SQL invariant suite seeds d012 (confirmed and currently in its
// readiness window) and c012 (provider-evidenced completed consultation).
// This test checks real PostgreSQL persistence, participant ownership and
// idempotency across independent API store handles, not in-memory fixtures.
func TestConsultationJourneyDurableReadAfterStoreRestart(t *testing.T) {
	dsn := os.Getenv("APGIC_CONNECTOR_DELIVERY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated signed connector integration database not configured")
	}
	ctx := context.Background()
	readyBooking := "00000000-0000-0000-0000-00000000d012"
	completedBooking := "00000000-0000-0000-0000-00000000c012"
	clientID := "00000000-0000-0000-0000-00000000b002"
	specialistID := "00000000-0000-0000-0000-00000000b001"
	otherID := "00000000-0000-0000-0000-00000000b003"

	store, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	readiness, err := store.ConfirmConsultationReady(ctx, readyBooking, clientID, time.Now().UTC())
	if err != nil || readiness == nil || readiness.ClientReadyAt == nil ||
		readiness.SpecialistReadyAt == nil {
		t.Fatalf("durable ready check: value=%+v error=%v", readiness, err)
	}
	// Exercise the full PostgreSQL join gate with a seeded active connector,
	// two distinct readiness acknowledgements and actual entitlement versions.
	// Repeat the same logical grant: the DB must persist one authorization
	// even when the API response or provider call is retried.
	grantKey, err := persistentid.New()
	if err != nil {t.Fatal(err)}
	grantKey = "r3-ready-join-" + grantKey
	prepared, err := store.PrepareCommunicationJoin(ctx,readyBooking,clientID,
		"00000000-0000-0000-0000-00000000c001",
		"communication-access-r3-ci-v1",grantKey,time.Now().UTC())
	if err != nil || prepared==nil || prepared.Decision.Decision!=communication.Allow ||
		prepared.Decision.CredentialRequest==nil {
		t.Fatalf("canonical ready and entitlement did not authorize join: decision=%+v err=%v",prepared,err)
	}
	grant:=communication.IssuedRoomGrant{
		ProviderCredentialRef:"provider/ci/"+grantKey,
		JoinURI:"https://media.example/room#ephemeral-test-only",
		ExpiresAt:prepared.Decision.CredentialRequest.ExpiresAt,
	}
	if err:=store.RecordCommunicationJoin(ctx,readyBooking,clientID,grantKey,*prepared,grant,time.Now().UTC());err!=nil {
		t.Fatalf("scoped canonical join not durable: %v",err)
	}
	if err:=store.RecordCommunicationJoin(ctx,readyBooking,clientID,grantKey,*prepared,grant,time.Now().UTC());err!=nil {
		t.Fatalf("idempotent join replay not accepted: %v",err)
	}
	var grants int
	if err:=store.db.QueryRowContext(ctx,`SELECT count(*) FROM communication_join_authorizations
		WHERE booking_id=$1::uuid AND identity_id=$2::uuid AND idempotency_key=$3`,
		readyBooking,clientID,grantKey).Scan(&grants);err!=nil||grants!=1 {
		t.Fatalf("canonical join idempotency broken: count=%d err=%v",grants,err)
	}
	if denied, err := store.ConfirmConsultationReady(ctx, readyBooking, otherID, time.Now().UTC()); err != nil || denied != nil {
		t.Fatalf("foreign principal confirmed readiness: value=%+v error=%v", denied, err)
	}
	if denied, err := store.ReadConsultationReady(ctx, readyBooking, otherID); err != nil || denied != nil {
		t.Fatalf("foreign principal read readiness: value=%+v error=%v", denied, err)
	}
	if _, err := store.ConfirmConsultationReady(ctx, completedBooking, clientID, time.Now().UTC()); !errors.Is(err, connector.ErrConsultationNotReady) {
		t.Fatalf("terminal booking accepted readiness: %v", err)
	}
	// A specialist can see provider-verified lifecycle but cannot create the
	// client's personal progress/satisfaction outcome.
	session, err := store.ReadConsultationResult(ctx, completedBooking, specialistID)
	if err != nil || session == nil || session.State != "COMPLETED" || session.CompletionEvidenceRef == "" {
		t.Fatalf("specialist cannot read provider completion: session=%+v error=%v", session, err)
	}
	anonymous, err := store.ReadConsultationResult(ctx, completedBooking, otherID)
	if err != nil || anonymous != nil {
		t.Fatalf("foreign principal read signed result: session=%+v error=%v", anonymous, err)
	}
	if denied, err := store.RecordConsultationOutcome(ctx, completedBooking, specialistID,
		"r3-specialist-forged-key-0001", 2, 4, time.Now().UTC()); err != nil || denied != nil {
		t.Fatalf("specialist forged client outcome: outcome=%+v error=%v", denied, err)
	}
	if _, err := store.RecordConsultationOutcome(ctx, readyBooking, clientID,
		"r3-premature-outcome-key-0001", 1, 5, time.Now().UTC()); !errors.Is(err, connector.ErrConsultationOutcomeNotReady) {
		t.Fatalf("unverified session accepted client outcome: %v", err)
	}
	key, err := persistentid.New()
	if err != nil {
		t.Fatal(err)
	}
	key = "r3-client-feedback-" + key
	original, err := store.RecordConsultationOutcome(ctx, completedBooking, clientID, key, 2, 4, time.Now().UTC())
	if err != nil || original == nil || original.Idempotent {
		t.Fatalf("failed first client outcome: %+v error=%v", original, err)
	}
	replayed, err := store.RecordConsultationOutcome(ctx, completedBooking, clientID, key, 2, 4, time.Now().UTC())
	if err != nil || replayed == nil || !replayed.Idempotent ||
		!replayed.ReportedAt.Equal(original.ReportedAt) {
		t.Fatalf("client outcome replay duplicated: %+v error=%v", replayed, err)
	}
	if _, err := store.RecordConsultationOutcome(ctx, completedBooking, clientID, key, -2, 4, time.Now().UTC()); !errors.Is(err, connector.ErrConsultationOutcomeConflict) {
		t.Fatalf("idempotency key was reused for a different outcome: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	// A distinct database pool must read exactly the same business truth.
	restarted, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	persistedReady, err := restarted.ReadConsultationReady(ctx, readyBooking, specialistID)
	if err != nil || persistedReady == nil || persistedReady.ClientReadyAt == nil || persistedReady.SpecialistReadyAt == nil {
		t.Fatalf("restart lost readiness: %+v error=%v", persistedReady, err)
	}
	latest, err := restarted.ReadConsultationOutcome(ctx, completedBooking, clientID)
	if err != nil || latest == nil || latest.Progress != 2 || latest.MatchSatisfaction != 4 {
		t.Fatalf("restart lost client-reported outcome: %+v error=%v", latest, err)
	}
	if private, err := restarted.ReadConsultationOutcome(ctx, completedBooking, specialistID); err != nil || private != nil {
		t.Fatalf("specialist accessed private client outcome: %+v error=%v", private, err)
	}
}
