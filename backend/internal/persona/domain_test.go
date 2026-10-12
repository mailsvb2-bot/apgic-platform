package persona
import ("errors";"testing")
func TestStartPrivacyAndSafetyInvariants(t *testing.T) {
 s:=Start{PersonaID:"test",Purpose:"LISTEN",Mode:"VOICE",Language:"ru",Style:"CALM",PrivacyMode:PrivateEphemeral}
 if err:=s.Validate();err!=nil{t.Fatal(err)}
 for _,bad:=range []Start{
  {PersonaID:"test",Purpose:"DIAGNOSE",Mode:"VOICE",Language:"ru",Style:"CALM",PrivacyMode:PrivateEphemeral},
  {PersonaID:"test",Purpose:"LISTEN",Mode:"VOICE",Language:"ru",Style:"CALM",PrivacyMode:"PUBLIC"},
  {PersonaID:"test",Purpose:"LISTEN",Mode:"VOICE",Language:"ru",Style:"CALM",PrivacyMode:"REMEMBER"},
  {PersonaID:"test",Purpose:"LISTEN",Mode:"HUMAN_IMPERSONATION",Language:"ru",Style:"CALM",PrivacyMode:PrivateEphemeral},
  {PersonaID:"test",Purpose:"LISTEN",Mode:"VOICE",Language:"../bad",Style:"CALM",PrivacyMode:PrivateEphemeral},
 } {if !errors.Is(bad.Validate(),ErrInvalidRequest){t.Errorf("accepted unsafe request: %+v",bad)}}
}
func TestLifecycleAndNoImplicitHandoff(t *testing.T) {
 if !AllowedTransition(StatePending,StateActive)||!AllowedTransition(StatePending,StateFailed)||!AllowedTransition(StateActive,StateEnded){t.Fatal("missing lifecycle edge")}
 for _,v:=range [][2]string{{StatePending,StateEnded},{StateFailed,StateActive},{StateEnded,StateActive},{StateActive,StatePending}}{if AllowedTransition(v[0],v[1]){t.Fatalf("invalid transition %v",v)}}
 projection:=SafeHandoffProjection(Session{ID:"a",ProviderSessionRef:"secret",Purpose:"LISTEN"})
 if len(projection)!=2||projection["next_action"]!="NEW_HELP_INTENT_REQUIRED"{t.Fatalf("unsafe handoff: %+v",projection)}
}
