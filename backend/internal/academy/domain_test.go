package academy

import (
 "errors"
 "testing"
)
func TestLessonValidationAndProviderBoundary(t *testing.T) {
 for _,l:= range []Lesson{
  {Kind: Reading,Ordinal:1,Title:"Read",Body:""},
  {Kind:Video,Ordinal:1,Title:"Watch",MediaAssetRef:"https://public.example/video.mp4"},
  {Kind:Assessment,Ordinal:1,Title:"Quiz",PassingScore:101,RubricVersion:"r1"},
  {Kind:"UNKNOWN",Ordinal:1,Title:"Unknown"},
 } { if !errors.Is(ValidateLesson(l),ErrInvalid){t.Fatalf("expected invalid lesson: %+v",l)} }
 if err:=ValidateLesson(Lesson{Kind:Reading,Ordinal:1,Title:"Read",Body:"Learning text"});err!=nil {t.Fatal(err)}
 if !errors.Is(CanMarkProgress(Video,false,false),ErrDenied){t.Fatal("client cannot self-attest playback")}
 if !errors.Is(CanMarkProgress(Assessment,false,false),ErrDenied){t.Fatal("client cannot self-grade assessment")}
 if err:=CanMarkProgress(Assessment,true,false);err!=nil {t.Fatal(err)}
 if !errors.Is(CanIssueCertificate(1,1,false,true),ErrEntitlement) {t.Fatal("revoked entitlement must block completion")}
 if !errors.Is(CanIssueCertificate(2,1,true,true),ErrState) {t.Fatal("incomplete learning must block")}
 if err:=CanIssueCertificate(2,2,true,true);err!=nil {t.Fatal(err)}
 if EvidenceCandidateStatus!="PENDING_VERIFICATION" {t.Fatal("qualification must never be automatic")}
}
