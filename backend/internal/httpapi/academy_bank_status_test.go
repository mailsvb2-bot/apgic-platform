package httpapi

import (
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"

 "github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type academyBankStatusStub struct{
 *academyStoreStub
 called int
 buyer string
}
func(s *academyBankStatusStub)AcademyReadBankOrderStatus(buyer,orderID string)(academy.BankOrderStatus,error){
 s.called++
 s.buyer=buyer
 return academy.BankOrderStatus{OrderID:orderID,CourseID:"22222222-2222-4222-8222-222222222222",PaymentState:"SENT",RequiresReconciliation:true},nil
}
func TestAcademyBankStatusRequiresSignedOwnerAndNeverClaimsPayment(t *testing.T){
 manager,err:=newClientSessionManager([]byte(strings.Repeat("k",32)),nil)
 if err!=nil{t.Fatal(err)}
 validOwner:="11111111-1111-4111-8111-111111111111"
 cookie,err:=manager.issue(validOwner)
 if err!=nil{t.Fatal(err)}
 store:=&academyBankStatusStub{academyStoreStub:&academyStoreStub{}}
 mux:=http.NewServeMux()
 registerAcademyBankOrderStatus(mux,store,manager,nil)
 route:="/v1/academy/bank/orders/33333333-3333-4333-8333-333333333333"
 anonymous:=httptest.NewRecorder()
 mux.ServeHTTP(anonymous,httptest.NewRequest(http.MethodGet,route,nil))
 if anonymous.Code==http.StatusOK||store.called!=0{
  t.Fatalf("unsigned user read private payment status %d",anonymous.Code)
 }
 signed:=httptest.NewRequest(http.MethodGet,route,nil)
 signed.AddCookie(cookie)
 response:=httptest.NewRecorder()
 mux.ServeHTTP(response,signed)
 if response.Code!=http.StatusOK||store.called!=1||store.buyer!=validOwner||
  !strings.Contains(response.Body.String(),`"requires_reconciliation":true`)||
  strings.Contains(response.Body.String(),`"entitlement_active":true`){
  t.Fatalf("buyer bank readback broken status=%d body=%s",response.Code,response.Body.String())
 }
}
