package httpapi

import (
 "bytes"
 "io"
 "net/http"
 "net/http/httptest"
 "testing"
 "time"
)

// Uses a real server and the production SLI wrapper. A long-lived video
// must outlive cmd/api's whole-response timeout while each write remains
// bounded by the streaming gateway's rolling deadline.
func TestAcademyVideoStreamingDeadlineOutlivesServerWriteTimeout(t *testing.T) {
 mux:=http.NewServeMux()
 mux.HandleFunc("/video",func(w http.ResponseWriter,r *http.Request){
  deadline:=newAcademyVideoResponseDeadline(w)
  deadline.extend(time.Second)
  writer:=&academyVideoLearnerWriter{w:w,deadline:deadline}
  w.Header().Set("Content-Type","video/mp4")
  if _,err:=writer.Write(bytes.Repeat([]byte("a"),64));err!=nil{return}
  if err:=writer.FlushError();err!=nil{return}
  time.Sleep(150*time.Millisecond)
  if _,err:=writer.Write(bytes.Repeat([]byte("b"),64));err!=nil{return}
  _=writer.FlushError()
 })
 server:=httptest.NewUnstartedServer(observeRuntimeSLI(mux,newRuntimeSLI()))
 server.Config.WriteTimeout=50*time.Millisecond
 server.Start()
 defer server.Close()
 response,err:=server.Client().Get(server.URL+"/video")
 if err!=nil{t.Fatal(err)}
 defer response.Body.Close()
 body,err:=io.ReadAll(response.Body)
 if err!=nil{t.Fatalf("video truncated despite active rolling deadline: %v",err)}
 if !bytes.Equal(body,append(bytes.Repeat([]byte("a"),64),bytes.Repeat([]byte("b"),64)...)){
  t.Fatalf("unexpected delivered bytes: %d",len(body))
 }
}
