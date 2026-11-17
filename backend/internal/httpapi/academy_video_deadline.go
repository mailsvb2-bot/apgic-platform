package httpapi

import (
 "net/http"
 "sync"
 "time"
)
const (
 academyVideoPreflightTimeout = 60*time.Second
 academyVideoWriteIdleTimeout = 30*time.Second
)
// A revocation and a deadline extension must never race: after revoke
// no writer can restore a live deadline.
type academyVideoResponseDeadline struct {
 mu sync.Mutex
 controller *http.ResponseController
 revoked bool
}
func newAcademyVideoResponseDeadline(w http.ResponseWriter)*academyVideoResponseDeadline{
 return &academyVideoResponseDeadline{controller:http.NewResponseController(w)}
}
func(d *academyVideoResponseDeadline)extend(budget time.Duration){
 d.mu.Lock();defer d.mu.Unlock()
 if !d.revoked{_ = d.controller.SetWriteDeadline(time.Now().Add(budget))}
}
func(d *academyVideoResponseDeadline)revoke(){
 d.mu.Lock();defer d.mu.Unlock()
 d.revoked=true
 _=d.controller.SetWriteDeadline(time.Now())
}
func(d *academyVideoResponseDeadline)wasRevoked()bool{
 d.mu.Lock();defer d.mu.Unlock();return d.revoked
}
type academyVideoLearnerWriter struct{
 w http.ResponseWriter
 deadline *academyVideoResponseDeadline
 pending int64
 delivered int64
}
func(lw *academyVideoLearnerWriter)Write(p []byte)(int,error){
 lw.deadline.extend(academyVideoWriteIdleTimeout)
 n,err:=lw.w.Write(p);lw.pending+=int64(n);return n,err
}
func(lw *academyVideoLearnerWriter)FlushError()error{
 lw.deadline.extend(academyVideoWriteIdleTimeout)
 if err:=lw.deadline.controller.Flush();err!=nil{return err}
 lw.delivered+=lw.pending
 lw.pending=0
 return nil
}
