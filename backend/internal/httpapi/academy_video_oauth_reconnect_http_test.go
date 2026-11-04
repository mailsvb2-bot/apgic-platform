package httpapi

import (
 "bytes"
 "context"
 "io"
 "net/http"
 "net/http/httptest"
 "strings"
 "sync/atomic"
 "testing"
 "time"
)

// Regression: OAuth reconnection must terminate a real downstream HTTP stream,
// not just make the next manifest request fail. This is socket-level evidence,
// not a substitute for canonical PostgreSQL Entitlement or live Yandex OAuth.
func TestAcademyVideoHTTPClientStopsOnOAuthReconnect(t *testing.T) {
 const chunk = 64 << 10
 release := make(chan struct{})
 upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request) {
  w.Header().Set("Content-Length","131072")
  _,_ = io.WriteString(w,strings.Repeat("a",chunk))
  w.(http.Flusher).Flush()
  select {
  case <-release: _,_ = io.WriteString(w,strings.Repeat("b",chunk))
  case <-r.Context().Done():
  }
 }))
 defer upstream.Close()
 source := &academyRotatingCredentialSource{}
 source.token.Store("original-token")
 gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request) {
  ctx,cancel := context.WithCancel(r.Context())
  defer cancel()
  req,err := http.NewRequestWithContext(ctx,http.MethodGet,upstream.URL,nil)
  if err != nil { http.Error(w,"unavailable",http.StatusBadGateway); return }
  response,err := upstream.Client().Do(req)
  if err != nil { http.Error(w,"unavailable",http.StatusBadGateway); return }
  defer response.Body.Close()
  authorized := func() bool {
   return academyVideoCloudConnectionStillAllowed(source,"author","YANDEX_DISK","original-token")
  }
  if !authorized() { http.Error(w,"denied",http.StatusForbidden); return }
  w.Header().Set("Content-Length","131072")
  w.Header().Set("Cache-Control","private, no-store")
  w.WriteHeader(http.StatusOK)
  done := make(chan struct{})
  go func() {
   defer close(done)
   academyWatchVideoEntitlement(ctx,func(){academyCancelRevokedVideoResponse(w,cancel)},5*time.Millisecond,authorized)
  }()
  defer func(){cancel(); <-done}()
  _ = academyCopyVideoAuthorized(w,response.Body,authorized)
 }))
 defer gateway.Close()
 client := gateway.Client()
 client.Timeout = 3*time.Second
 response,err := client.Get(gateway.URL)
 if err != nil {t.Fatal(err)}
 defer response.Body.Close()
 if response.StatusCode != http.StatusOK {t.Fatalf("status=%d",response.StatusCode)}
 first := make([]byte,chunk)
 if _,err := io.ReadFull(response.Body,first); err != nil {t.Fatalf("first chunk: %v",err)}
 if !bytes.Equal(first,bytes.Repeat([]byte("a"),chunk)) {t.Fatal("wrong first chunk")}
 source.token.Store("replacement-token")
 // Wait for watcher cancellation before releasing upstream to avoid racing
 // the credential update against the next authorized read.
 time.Sleep(30*time.Millisecond)
 close(release)
 remainder,err := io.ReadAll(response.Body)
 if len(remainder)!=0 || err==nil {t.Fatalf("reconnected account leaked %d bytes, err=%v",len(remainder),err)}
}
