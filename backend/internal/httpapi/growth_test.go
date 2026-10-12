package httpapi

import (
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "time"

 "github.com/mailsvb2-bot/apgic-platform/backend/internal/growth"
)

type growthFixture struct{ issued,captured int }
func (f *growthFixture) IssueSpecialistLink(_,offer string,until time.Time)(growth.Link,error){
 f.issued++
 return growth.Link{ID:"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",OfferID:offer,TopicID:"sleep",Source:growth.SourceSpecialist,ExpiresAt:until,PolicyVersion:growth.LinkPolicyVersion,State:"ACTIVE"},nil
}
func (f *growthFixture) CaptureTouch(_,id string,at time.Time)(growth.Touch,error){
 f.captured++;return growth.Touch{ID:"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",LinkID:id,Source:growth.SourceSpecialist,TopicID:"sleep",RecordedAt:at},nil
}
func (f *growthFixture) RevokeSpecialistLink(_, _ string)error{return nil}
func (f *growthFixture) SpecialistGrowthReport(_ string)([]growth.Report,error){return []growth.Report{},nil}

func TestGrowthSignedLinkAndFirstPartyIdentity(t *testing.T){
 now:=time.Date(2026,10,12,10,0,0,0,time.UTC)
 key:=[]byte(strings.Repeat("k",32))
 f:=&growthFixture{}
 sessions,_:=newClientSessionManager(key,func()time.Time{return now})
 h:=New(Options{Growth:f,GrowthSigningKey:key,ClientSessionKey:key,Now:func()time.Time{return now}})
 unauthorized:=httptest.NewRecorder()
 h.ServeHTTP(unauthorized,httptest.NewRequest("POST","/v1/specialist/growth-links",strings.NewReader("{\"offer_id\":\"offer\"}")))
 if unauthorized.Code!=http.StatusUnauthorized||f.issued!=0{t.Fatalf("unauthorized=%d",unauthorized.Code)}
 cookie,_:=sessions.issue("11111111-1111-4111-8111-111111111111")
 mint:=httptest.NewRequest("POST","/v1/specialist/growth-links",strings.NewReader("{\"offer_id\":\"offer\"}"))
 mint.AddCookie(cookie)
 response:=httptest.NewRecorder();h.ServeHTTP(response,mint)
 if response.Code!=http.StatusCreated{t.Fatalf("mint=%d %s",response.Code,response.Body.String())}
 var link growth.Link
 if err:=json.Unmarshal(response.Body.Bytes(),&link);err!=nil{t.Fatal(err)}
 signer,_:=growth.NewSigner(key);ref,err:=signer.Verify(link.Token,now)
 if err!=nil||ref!=link.ID{t.Fatalf("signed token %q %v",ref,err)}
 captureBody,_:=json.Marshal(map[string]string{"token":link.Token})
 visitor:=httptest.NewRecorder()
 h.ServeHTTP(visitor,httptest.NewRequest("POST","/v1/growth/touches",strings.NewReader(string(captureBody))))
 if visitor.Code!=http.StatusOK||f.captured!=1||len(visitor.Result().Cookies())!=1{
  t.Fatalf("capture=%d count=%d cookies=%d body=%s",visitor.Code,f.captured,len(visitor.Result().Cookies()),visitor.Body.String())
 }
 tampered:=httptest.NewRecorder()
 h.ServeHTTP(tampered,httptest.NewRequest("POST","/v1/growth/touches",strings.NewReader("{\"token\":\""+link.Token+"bad\"}")))
 if tampered.Code!=http.StatusBadRequest||f.captured!=1{t.Fatalf("tamper=%d count=%d",tampered.Code,f.captured)}
}
func TestGrowthFailsClosedWithoutKey(t *testing.T){
 h:=New(Options{Growth:&growthFixture{},ClientSessionKey:[]byte(strings.Repeat("k",32))})
 r:=httptest.NewRecorder();h.ServeHTTP(r,httptest.NewRequest("POST","/v1/growth/touches",strings.NewReader("{}")))
 if r.Code!=http.StatusServiceUnavailable{t.Fatalf("unconfigured growth=%d",r.Code)}
}
