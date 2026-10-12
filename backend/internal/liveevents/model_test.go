package liveevents
import (
 "errors"
 "testing"
 "time"
)
func valid(now time.Time) DraftInput {return DraftInput{Title:"Групповая встреча",Description:"Обсуждаем практику",Format:Group,StartsAt:now.Add(time.Hour),EndsAt:now.Add(2*time.Hour),Capacity:15}}
func TestDraftValidationAndNoShadowCheckout(t *testing.T){
 now:=time.Date(2026,10,12,12,0,0,0,time.UTC)
 in:=valid(now)
 got,err:=NormalizeDraft(in,now);if err!=nil||got.Currency!="RUB"{t.Fatalf("valid draft %+v: %v",got,err)}
 cases:=[]struct{name string; change func(*DraftInput); want error}{
 {"empty name",func(x *DraftInput){x.Title=" "},ErrInvalid},
 {"unknown format",func(x *DraftInput){x.Format="ROOM"},ErrInvalid},
 {"past",func(x *DraftInput){x.StartsAt=now.Add(-time.Hour)},ErrInvalid},
 {"inverted",func(x *DraftInput){x.EndsAt=x.StartsAt},ErrInvalid},
 {"too long",func(x *DraftInput){x.EndsAt=x.StartsAt.Add(9*time.Hour)},ErrInvalid},
 {"zero seats",func(x *DraftInput){x.Capacity=0},ErrInvalid},
 {"above cap",func(x *DraftInput){x.Capacity=1001},ErrInvalid},
 {"fake paid",func(x *DraftInput){x.PriceMinor=100},ErrPaymentRequired},
 {"unknown currency",func(x *DraftInput){x.Currency="USD"},ErrInvalid},
 }
 for _,tt:=range cases {t.Run(tt.name,func(t *testing.T){v:=valid(now);tt.change(&v);_,err:=NormalizeDraft(v,now);if !errors.Is(err,tt.want){t.Fatalf("got %v want %v",err,tt.want)}})}
}
func TestLifecycleNoTerminalResurrection(t *testing.T){
 allowed:=map[[2]State]bool{
 {Draft,Published}:true,{Draft,Cancelled}:true,
 {Published,Live}:true,{Published,Cancelled}:true,
 {Live,Completed}:true,{Live,Cancelled}:true,
 }
 states:=[]State{Draft,Published,Live,Completed,Cancelled}
 for _,from:=range states {for _,to:=range states{
  if CanTransition(from,to)!=allowed[[2]State{from,to}]{t.Fatalf("%s -> %s incorrect",from,to)}
 }}
}
func TestJoinRequiresRegistrationWindowAndVerifiedProvider(t *testing.T){
 start:=time.Date(2026,10,12,12,0,0,0,time.UTC);end:=start.Add(time.Hour)
 cases:=[]struct{state State;registered bool;at time.Time;provider bool;reason string;allow bool}{
 {Published,false,start,true,"LIVE_REGISTRATION_REQUIRED",false},
 {Draft,true,start,true,"LIVE_EVENT_NOT_JOINABLE",false},
 {Completed,true,start,true,"LIVE_EVENT_NOT_JOINABLE",false},
 {Live,true,start.Add(-16*time.Minute),true,"LIVE_OUTSIDE_JOIN_WINDOW",false},
 {Live,true,start,false,"LIVE_PROVIDER_UNAVAILABLE",false},
 {Live,true,start,true,"LIVE_JOIN_ELIGIBLE",true},
 {Published,true,end.Add(15*time.Minute),true,"LIVE_OUTSIDE_JOIN_WINDOW",false},
 }
 for _,tc:=range cases {x:=CanJoin(tc.state,tc.registered,tc.at,start,end,tc.provider);if x.Allowed!=tc.allow||x.ReasonCode!=tc.reason {t.Errorf("%+v => %+v",tc,x)}}
 if CanRegister(Draft,start.Add(-time.Hour),start)||CanRegister(Published,start,start)||!CanRegister(Published,start.Add(-time.Minute),start){t.Fatal("registration window")}
}
