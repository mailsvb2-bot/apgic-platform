package main

import (
 "bytes"
 "encoding/json"
 "fmt"
 "net/http"
 "net/http/httptest"
 "strings"
 "sync/atomic"
 "time"
 "testing"
)

func TestRealModelBackedPersonaConversationAndEphemeralDeletion(t *testing.T) {
 var requests atomic.Int32
 var seenHistory atomic.Int32
 llm:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.Header.Get("Authorization")!="Bearer model-secret-for-test" {t.Error("model credential missing")}
  var req struct{Messages []message `json:"messages"`}
  if json.NewDecoder(r.Body).Decode(&req)!=nil {t.Error("invalid model input")}
  seenHistory.Store(int32(len(req.Messages)));n:=requests.Add(1)
  w.Header().Set("Content-Type","application/json")
  fmt.Fprintf(w,`{"choices":[{"message":{"role":"assistant","content":"Настоящий ответ модели номер %d"},"finish_reason":"stop"}]}`,n)
 }))
 defer llm.Close()
 provider:=newService(config{token:strings.Repeat("x",32),modelURL:llm.URL+"/v1/chat/completions",modelKey:"model-secret-for-test",model:"evaluated",policy:"reviewed-v1",client:llm.Client()})
 api:=httptest.NewServer(provider.handler());defer api.Close()
 post:=func(path string,data any,auth bool)*http.Response {
  t.Helper();body,err:=json.Marshal(data);if err!=nil{t.Fatal(err)}
  req,err:=http.NewRequest("POST",api.URL+path,bytes.NewReader(body));if err!=nil{t.Fatal(err)}
  req.Header.Set("Content-Type","application/json")
  if auth{req.Header.Set("Authorization","Bearer "+strings.Repeat("x",32))}
  res,err:=http.DefaultClient.Do(req);if err!=nil{t.Fatal(err)}
  return res
 }
 opening:=openRequest{APGICSessionID:"00000000-0000-0000-0000-000000000301",PersonaID:"00000000-0000-0000-0000-000000000201",Purpose:"LISTEN",PrivacyMode:"PRIVATE_EPHEMERAL",Mode:"TEXT",Style:"CALM",Language:"ru",VoiceProfile:"rights-cleared",VisualProfile:"no-real-person",PolicyVersion:"reviewed-v1"}
 denied:=post("/v1/persona-sessions",opening,false);denied.Body.Close()
 if denied.StatusCode!=401{t.Fatal("unauthenticated session opened")}
 created:=post("/v1/persona-sessions",opening,true)
 if created.StatusCode!=201{t.Fatal("open failed",created.StatusCode)}
 var ticket struct{SessionRef string `json:"session_ref"`;LaunchURL string `json:"launch_url"`;Mode string `json:"mode"`}
 if err:=json.NewDecoder(created.Body).Decode(&ticket);err!=nil{t.Fatal(err)};created.Body.Close()
 if len(ticket.SessionRef)!=64||ticket.LaunchURL!=""||ticket.Mode!="TEXT"{t.Fatalf("unexpected ticket: %+v",ticket)}
 for i:=1;i<=2;i++ {
  res:=post("/v1/persona-sessions/turns",map[string]string{"session_ref":ticket.SessionRef,"message":"Привет"},true)
  var out struct{Reply string `json:"reply"`;Source string `json:"source"`}
  err:=json.NewDecoder(res.Body).Decode(&out);res.Body.Close()
  if err!=nil||res.StatusCode!=200||out.Source!="MODEL_GENERATED"||out.Reply!=fmt.Sprintf("Настоящий ответ модели номер %d",i){
   t.Fatalf("real turn %d failed: %+v %v (%d)",i,out,err,res.StatusCode)
  }
 }
 if requests.Load()!=2||seenHistory.Load()!=4{t.Fatalf("no real multi-turn context: calls=%d last history=%d",requests.Load(),seenHistory.Load())}
 closed:=post("/v1/persona-sessions/close",map[string]string{"session_ref":ticket.SessionRef},true);closed.Body.Close()
 if closed.StatusCode!=200{t.Fatal("close failed")}
 provider.mu.Lock();left:=len(provider.sessions);provider.mu.Unlock()
 if left!=0{t.Fatal("ephemeral content was not deleted")}
 revoked:=post("/v1/persona-sessions/turns",map[string]string{"session_ref":ticket.SessionRef,"message":"Ещё"},true)
 revoked.Body.Close();if revoked.StatusCode!=410{t.Fatal("revoked provider still talking",revoked.StatusCode)}
 if requests.Load()!=2{t.Fatal("inference after deletion")}
}

func TestPersonaProviderRejectsModelOutageAndLimits(t *testing.T) {
 model:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,_ *http.Request){http.Error(w,"unavailable",503)}));defer model.Close()
 provider:=newService(config{token:strings.Repeat("s",32),modelURL:model.URL,modelKey:"model-secret-for-test",model:"evaluated",policy:"v1",client:model.Client()})
 ref:=strings.Repeat("a",64)
 provider.sessions[ref]=&session{mode:"TEXT",expires:time.Now().Add(sessionTTL)}
 req:=httptest.NewRequest("POST","/v1/persona-sessions/turns",bytes.NewBufferString(`{"session_ref":"`+ref+`","message":"Привет"}`))
 req.Header.Set("Authorization","Bearer "+strings.Repeat("s",32))
 rec:=httptest.NewRecorder();provider.handler().ServeHTTP(rec,req)
 if rec.Code!=503||strings.Contains(rec.Body.String(),"MODEL_GENERATED"){t.Fatalf("fake reply after model outage: %d %s",rec.Code,rec.Body.String())}
 provider.sessions[ref].turns=maxSessionTurns
 req2:=httptest.NewRequest("POST","/v1/persona-sessions/turns",bytes.NewBufferString(`{"session_ref":"`+ref+`","message":"Ещё"}`))
 req2.Header.Set("Authorization","Bearer "+strings.Repeat("s",32))
 rec2:=httptest.NewRecorder();provider.handler().ServeHTTP(rec2,req2)
 if rec2.Code!=429{t.Fatalf("turn budget bypassed: %d",rec2.Code)}
}
