package consultation

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func newSession(t *testing.T) (*Session, time.Time) {
	t.Helper()
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	session, err := New(
		"consultation-1",
		"booking-1",
		"client-1",
		"specialist-1",
		"communication-provider-1",
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	return session, now
}

func participantFact(id string, kind FactType, role ParticipantRole, identity string, at time.Time) Fact {
	return Fact{
		ID: id, IdempotencyKey: "idem-" + id,
		Type: kind, Role: role, IdentityID: identity,
		ProviderInstanceID: "communication-provider-1",
		ProviderReference: "room/1", EvidenceRef: "provider-evidence/" + id,
		OccurredAt: at,
	}
}

func systemFact(id string, kind FactType, at time.Time) Fact {
	return Fact{
		ID: id, IdempotencyKey: "idem-" + id,
		Type: kind, Role: RoleSystem,
		ProviderInstanceID: "communication-provider-1",
		ProviderReference: "room/1", EvidenceRef: "provider-evidence/" + id,
		OccurredAt: at,
	}
}

func TestLifecycleRequiresBothParticipantJoinsBeforeStart(t *testing.T) {
	session, now := newSession(t)
	if err := session.RecordFact(participantFact("client-join", FactJoined, RoleClient, "client-1", now.Add(time.Minute))); err != nil {
		t.Fatal(err)
	}
	if err := session.RecordFact(systemFact("start-too-early", FactStarted, now.Add(2*time.Minute))); !errors.Is(err, ErrTransitionDenied) {
		t.Fatalf("start with one participant err=%v", err)
	}
	if err := session.RecordFact(participantFact("specialist-join", FactJoined, RoleSpecialist, "specialist-1", now.Add(3*time.Minute))); err != nil {
		t.Fatal(err)
	}
	if session.State != StateReady {
		t.Fatalf("state = %s", session.State)
	}
	if err := session.RecordFact(systemFact("start", FactStarted, now.Add(4*time.Minute))); err != nil {
		t.Fatal(err)
	}
	if session.State != StateInProgress {
		t.Fatalf("state = %s", session.State)
	}
}

func TestCompletionRequiresExplicitProviderEvidenceNotTimer(t *testing.T) {
	session, now := newSession(t)
	if err := session.RecordFact(participantFact("client-join", FactJoined, RoleClient, "client-1", now.Add(time.Minute))); err != nil {
		t.Fatal(err)
	}
	if err := session.RecordFact(participantFact("specialist-join", FactJoined, RoleSpecialist, "specialist-1", now.Add(2*time.Minute))); err != nil {
		t.Fatal(err)
	}
	if err := session.RecordFact(systemFact("start", FactStarted, now.Add(3*time.Minute))); err != nil {
		t.Fatal(err)
	}

	if err := session.Complete(CompletionEvidence{
		ID: "completion-1", IdempotencyKey: "completion-idem-1",
		ObservedAt: now.Add(2 * time.Hour),
	}); !errors.Is(err, ErrCompletionEvidence) {
		t.Fatalf("timer-only completion must fail, got %v", err)
	}
	if session.State != StateInProgress {
		t.Fatalf("timer-only completion mutated state to %s", session.State)
	}

	if err := session.Complete(CompletionEvidence{
		ID: "completion-2", IdempotencyKey: "completion-idem-2",
		ProviderInstanceID: "communication-provider-1",
		ProviderReference: "room/1",
		EvidenceRef:       "provider-evidence/room-ended",
		ObservedAt:        now.Add(2*time.Hour + time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	if session.State != StateCompleted {
		t.Fatalf("state = %s", session.State)
	}
}

func TestTechnicalFailureIsExplicitAndCannotBecomeCompletion(t *testing.T) {
	session, now := newSession(t)
	if err := session.RecordFact(participantFact("client-join", FactJoined, RoleClient, "client-1", now.Add(time.Minute))); err != nil {
		t.Fatal(err)
	}
	if err := session.RecordFact(participantFact("specialist-join", FactJoined, RoleSpecialist, "specialist-1", now.Add(2*time.Minute))); err != nil {
		t.Fatal(err)
	}
	if err := session.RecordFact(systemFact("start", FactStarted, now.Add(3*time.Minute))); err != nil {
		t.Fatal(err)
	}
	if err := session.RecordFact(systemFact("technical-failure", FactTechnicalFailure, now.Add(4*time.Minute))); err != nil {
		t.Fatal(err)
	}
	if session.State != StateTechnicalFailure {
		t.Fatalf("state = %s", session.State)
	}
	if err := session.Complete(CompletionEvidence{
		ID: "completion", IdempotencyKey: "completion-idem",
		ProviderInstanceID: "communication-provider-1",
		ProviderReference: "room/1", EvidenceRef: "provider-evidence/end",
		ObservedAt: now.Add(5 * time.Minute),
	}); !errors.Is(err, ErrTransitionDenied) {
		t.Fatalf("technical failure must not silently complete, got %v", err)
	}
}

func TestLifecycleFactsAreIdempotentAndContentMinimized(t *testing.T) {
	session, now := newSession(t)
	fact := participantFact("client-ready", FactReady, RoleClient, "client-1", now.Add(time.Minute))
	if err := session.RecordFact(fact); err != nil {
		t.Fatal(err)
	}
	if err := session.RecordFact(fact); !errors.Is(err, ErrDuplicateFact) {
		t.Fatalf("duplicate fact err=%v", err)
	}

	for _, got := range session.Facts() {
		if got.EvidenceRef == "" || got.ProviderReference == "" {
			t.Fatalf("fact lost minimal lifecycle evidence: %#v", got)
		}
	}
}


func TestLifecycleFactDoesNotContainRawSessionContentFields(t *testing.T) {
	typ := reflect.TypeOf(Fact{})
	for _, forbidden := range []string{
		"Transcript", "RawTranscript", "Audio", "Video", "Media",
		"RawContent", "Recording", "AvatarMedia",
	} {
		if _, ok := typ.FieldByName(forbidden); ok {
			t.Fatalf("consultation business fact must not contain raw session content field %s", forbidden)
		}
	}
}
