package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/communication"
)

type roomJoinStoreStub struct {
	produced *communication.JoinPreparation
	writes int
	failWrite bool
}
func (f *roomJoinStoreStub) PrepareCommunicationJoin(_ context.Context, bookingID, identityID, providerID, policy, key string, now time.Time) (*communication.JoinPreparation,error) {
	if bookingID!=journeyBookingID || identityID!=journeyIdentityID{return nil,nil}
	return f.produced,nil
}
func (f *roomJoinStoreStub) RecordCommunicationJoin(_ context.Context, b, i, k string, p communication.JoinPreparation, grant communication.IssuedRoomGrant, now time.Time)error{
	if f.failWrite{return errors.New("database down")}
	f.writes++
	return nil
}
type roomJoinIssuerStub struct{calls int}
func (i *roomJoinIssuerStub) IssueRoomJoin(_ context.Context, request communication.CredentialRequest)(communication.IssuedRoomGrant,error){
	i.calls++
	return communication.IssuedRoomGrant{ProviderCredentialRef:"provider/ref-1",JoinURI:"https://media.example/room/one",ExpiresAt:request.ExpiresAt},nil
}
func TestConsultationJoinRequiresTrustedPrincipalPolicyAndDurableDecision(t *testing.T){
	key:=[]byte("consultation-test-join-long-signed-cookie-secret")
	manager,err:=newClientSessionManager(key,time.Now)
	if err!=nil{t.Fatal(err)}
	cookie,err:=manager.issue(journeyIdentityID)
	if err!=nil{t.Fatal(err)}
	expiry:=time.Now().UTC().Add(3*time.Minute)
	store:=&roomJoinStoreStub{produced:&communication.JoinPreparation{
		Role:communication.RoleClient,EntitlementVersionID:"entitlement",
		Decision:communication.JoinDecision{Decision:communication.Allow,ReasonCode:communication.ReasonAllowed,
			ProviderInstanceID:"00000000-0000-4000-8000-000000000003",
			PolicyVersion:"join-v1",
			CredentialRequest:&communication.CredentialRequest{
				ProviderInstanceID:"00000000-0000-4000-8000-000000000003",
				BookingID:journeyBookingID,IdentityID:journeyIdentityID,
				Role:communication.RoleClient,Scope:"ROOM_JOIN",
				IdempotencyKey:"join-client-idempotent-0001",ExpiresAt:expiry,
			}},
	}}
	issuer:=&roomJoinIssuerStub{}
	h:=New(Options{ClientSessionKey:key,ConsultationJoin:ConsultationJoinOptions{
		Store:store,Issuer:issuer,
		ProviderInstanceID:"00000000-0000-4000-8000-000000000003",PolicyVersion:"join-v1",
	}})
	request:=func(signed,csrf bool)*httptest.ResponseRecorder{
		req:=httptest.NewRequest(http.MethodPost,"/v1/consultations/"+journeyBookingID+"/join",nil)
		if signed{req.AddCookie(cookie)}
		if csrf{req.Header.Set("X-APGIC-Request","consultation-v1")}
		req.Header.Set("Idempotency-Key","join-client-idempotent-0001")
		result:=httptest.NewRecorder();h.ServeHTTP(result,req)
		return result
	}
	if resp:=request(false,true);resp.Code!=http.StatusUnauthorized || issuer.calls!=0 {
		t.Fatalf("anonymous room join issued: %d",resp.Code)
	}
	if resp:=request(true,false);resp.Code!=http.StatusForbidden || issuer.calls!=0 {
		t.Fatalf("forged HTML form join issued: %d",resp.Code)
	}
	if resp:=request(true,true);resp.Code!=http.StatusOK || issuer.calls!=1 || store.writes!=1 {
		t.Fatalf("confirmed room join failed: status=%d calls=%d writes=%d body=%s",
			resp.Code,issuer.calls,store.writes,resp.Body.String())
	}else{
		var grant map[string]any
		if err:=json.Unmarshal(resp.Body.Bytes(),&grant);err!=nil{t.Fatal(err)}
		if grant["credential_scope"]!="ROOM_JOIN" || grant["join_uri"]!="https://media.example/room/one" {
			t.Fatalf("invalid room grant: %#v",grant)
		}
	}
	store.failWrite=true
	if resp:=request(true,true);resp.Code!=http.StatusConflict {
		t.Fatalf("room grant leaked despite failed canonical decision: %d",resp.Code)
	}else if strings.Contains(resp.Body.String(),"https://media.example") {
		t.Fatalf("room URL leaked after failed DB persistence: %s",resp.Body.String())
	}
	store.failWrite=false
	store.produced.Decision.Decision=communication.Deny
	store.produced.Decision.ReasonCode=communication.ReasonEntitlementMissing
	store.produced.Decision.CredentialRequest=nil
	n:=issuer.calls
	if resp:=request(true,true);resp.Code!=http.StatusForbidden || issuer.calls!=n {
		t.Fatalf("missing entitlement did not block issuer: %d calls=%d",resp.Code,issuer.calls)
	}
}
