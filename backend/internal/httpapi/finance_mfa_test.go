package httpapi

import (
 "context"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "time"
)

type stubFinanceMFA struct {
 accepted bool
 calls int
 identity string
 session string
}
func (s *stubFinanceMFA) VerifyFinanceMFA(_ context.Context,identity,session,code string,_ [32]byte,_ time.Time)(bool,error){
 s.calls++
 s.identity=identity
 s.session=session
 return s.accepted&&code=="123456",nil
}
func TestFinanceMFAOnlyIssuesStepUpAfterRealFactorAndExactSession(t *testing.T){
 now:=time.Date(2026,10,18,12,0,0,0,time.UTC)
 key:=[]byte(strings.Repeat("s",32))
 sessions,err:=newClientSessionManager(key,func()time.Time{return now})
 if err!=nil {t.Fatal(err)}
 manager,err:=newStepUpManager(key,func()time.Time{return now})
 if err!=nil {t.Fatal(err)}
 var mfaKey [32]byte
 mfaKey[0]=1
 factor:=&stubFinanceMFA{}
 mux:=http.NewServeMux()
 registerFinanceMFA(mux,factor,sessions,manager,mfaKey,nil,func()time.Time{return now})
 session,err:=sessions.issue("11111111-1111-4111-8111-111111111111")
 if err!=nil{t.Fatal(err)}
 request:=func(code string)*httptest.ResponseRecorder{
  req:=httptest.NewRequest(http.MethodPost,"https://apgic.ru/v1/finance/mfa/verify",strings.NewReader(`{"code":"`+code+`"}`))
  req.AddCookie(session)
  req.Header.Set("Content-Type","application/json")
  req.Header.Set("X-APGIC-MFA","totp-v1")
  w:=httptest.NewRecorder()
  mux.ServeHTTP(w,req)
  return w
 }
 failed:=request("000000")
 if failed.Code!=http.StatusForbidden {t.Fatalf("rejected factor status=%d",failed.Code)}
 if len(failed.Result().Cookies())!=0 {t.Fatal("failed factor minted a step-up cookie")}
 factor.accepted=true
 success:=request("123456")
 if success.Code!=http.StatusNoContent {t.Fatalf("correct factor status=%d body=%s",success.Code,success.Body.String())}
 issued:=success.Result().Cookies()
 if len(issued)!=1||issued[0].Name!=stepUpCookieName||!issued[0].HttpOnly||!issued[0].Secure{
  t.Fatalf("missing secure session-bound MFA cookie: %#v",issued)
 }
 withFactor:=httptest.NewRequest(http.MethodPost,"https://apgic.ru/test",nil)
 withFactor.AddCookie(session)
 withFactor.AddCookie(issued[0])
 identity,reference,err:=sessions.identityAndReferenceFromRequest(withFactor)
 if err!=nil {t.Fatal(err)}
 if result:=manager.evidenceFromRequest(withFactor,identity,reference);result==nil||result.Method!="TOTP"{
  t.Fatal("valid same-session MFA evidence not accepted")
 }
 second,err:=sessions.issue(identity)
 if err!=nil {t.Fatal(err)}
 wrongSession:=httptest.NewRequest(http.MethodPost,"https://apgic.ru/test",nil)
 wrongSession.AddCookie(second)
 wrongSession.AddCookie(issued[0])
 _,secondRef,err:=sessions.identityAndReferenceFromRequest(wrongSession)
 if err!=nil {t.Fatal(err)}
 if manager.evidenceFromRequest(wrongSession,identity,secondRef)!=nil{
  t.Fatal("MFA crossed session boundary")
 }
 if factor.calls!=2||factor.session!=reference||factor.identity!=identity{
  t.Fatal("factor verifier did not use trusted identity and session")
 }
}
