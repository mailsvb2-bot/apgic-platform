package persona

import (
	"errors"
	"testing"
)

func TestStartPrivacyAndSafetyInvariants(t *testing.T) {
	if ValidID("../../../x") || ValidID("") || ValidID("not-uuid") {
		t.Fatal("untrusted persona IDs must be rejected before storage access")
	}

	s := Start{PersonaID: "00000000-0000-0000-0000-000000000101", Purpose: "LISTEN", Mode: "VOICE", Language: "ru", Style: "CALM", PrivacyMode: PrivateEphemeral}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Start{
		{PersonaID: "00000000-0000-0000-0000-000000000101", Purpose: "DIAGNOSE", Mode: "VOICE", Language: "ru", Style: "CALM", PrivacyMode: PrivateEphemeral},
		{PersonaID: "00000000-0000-0000-0000-000000000101", Purpose: "LISTEN", Mode: "VOICE", Language: "ru", Style: "CALM", PrivacyMode: "PUBLIC"},
		{PersonaID: "00000000-0000-0000-0000-000000000101", Purpose: "LISTEN", Mode: "VOICE", Language: "ru", Style: "CALM", PrivacyMode: "REMEMBER"},
		{PersonaID: "00000000-0000-0000-0000-000000000101", Purpose: "LISTEN", Mode: "HUMAN_IMPERSONATION", Language: "ru", Style: "CALM", PrivacyMode: PrivateEphemeral},
		{PersonaID: "00000000-0000-0000-0000-000000000101", Purpose: "LISTEN", Mode: "VOICE", Language: "../bad", Style: "CALM", PrivacyMode: PrivateEphemeral},
	} {
		if !errors.Is(bad.Validate(), ErrInvalidRequest) {
			t.Errorf("accepted unsafe request: %+v", bad)
		}
	}
}
func TestLifecycleAndNoImplicitHandoff(t *testing.T) {
	if !AllowedTransition(StatePending, StateActive) || !AllowedTransition(StatePending, StateFailed) || !AllowedTransition(StateActive, StateEnded) {
		t.Fatal("missing lifecycle edge")
	}
	for _, v := range [][2]string{{StatePending, StateEnded}, {StateFailed, StateActive}, {StateEnded, StateActive}, {StateActive, StatePending}} {
		if AllowedTransition(v[0], v[1]) {
			t.Fatalf("invalid transition %v", v)
		}
	}
	projection := SafeHandoffProjection(Session{ID: "a", ProviderSessionRef: "secret", Purpose: "LISTEN"})
	if len(projection) != 2 || projection["next_action"] != "NEW_HELP_INTENT_REQUIRED" {
		t.Fatalf("unsafe handoff: %+v", projection)
	}
}
