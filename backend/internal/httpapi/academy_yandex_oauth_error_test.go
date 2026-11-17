package httpapi

import ("net/url";"testing")
func TestAcademyYandexOAuthProviderErrors(t *testing.T) {
 state:=cloudOAuthState{Nonce:"testnonce",ReturnCourse:"11111111-1111-4111-8111-111111111111",ReturnVersion:"22222222-2222-4222-8222-222222222222"}
 dest,ok:=cloudOAuthProviderError("error=access_denied&state=testnonce",state,true)
 if !ok || dest!="/academy?cloud_error=denied&course_id=11111111-1111-4111-8111-111111111111&version_id=22222222-2222-4222-8222-222222222222" {t.Fatalf("destination=%q ok=%v",dest,ok)}
 for _,query:=range []string{"error=access_denied&code=abc","error=access_denied&redirect=https://evil.example","error=%3Cscript%3E","error=access_denied&error=server_error"}{
  if d,ok:=cloudOAuthProviderError(query,state,true);ok{t.Fatalf("unsafe redirect: %q",d)}
 }
 if d,ok:=cloudOAuthProviderError("error=access_denied&state=foreign",state,true);!ok{
  t.Fatal("valid provider cancellation must be recognized")
 }else if parsed,_:=url.Parse(d);parsed.Query().Has("course_id"){t.Fatal("unmatched state exposed draft IDs")}
}
