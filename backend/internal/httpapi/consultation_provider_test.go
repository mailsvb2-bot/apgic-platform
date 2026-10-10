package httpapi

import (
    "context"
    "crypto/ed25519"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
    "time"

    "github.com/mailsvb2-bot/apgic-platform/backend/internal/connector"
)

type testConsultationProvider struct {
    applied int
    read int
}

func (f *testConsultationProvider) ApplyConsultationProviderWebhook(_ context.Context,_ connector.WebhookEnvelope,_ connector.WebhookPublicKeyResolver) (connector.DeliveryDecision,error) {
    f.applied++
    return connector.DeliveryApply,nil
}
func (f *testConsultationProvider) ReadConsultationResult(_ context.Context,_,_ string) (*connector.ConsultationResult,error) {
    f.read++
    return &connector.ConsultationResult{BookingID:"test",State:"COMPLETED"},nil
}

func TestConsultationWebhookRequiresVerifiedProviderSignatureAndReadRequiresSession(t *testing.T) {
    public,private,err:=ed25519.GenerateKey(nil)
    if err!=nil {t.Fatal(err)}
    keys:=providerWebhookKeyMap{"provider-1/key-1":public}
    fake:=&testConsultationProvider{}
    handler:=New(Options{ConsultationProvider:fake,ProviderWebhookKeys:keys})
    payload:=json.RawMessage(`{"booking_id":"00000000-0000-4000-8000-000000000001","fact_type":"ENDED","role":"SYSTEM","provider_reference":"room-1","evidence_ref":"provider/end-1"}`)
    event:=connector.WebhookEnvelope{
        ConnectorInstanceID:"provider-1",ExternalEventID:"event-1",
        StreamID:"consultation/00000000-0000-4000-8000-000000000001",
        Sequence:1,KeyID:"key-1",OccurredAt:time.Now().UTC(),
        Payload:payload,
    }
    unsignedJSON,err:=json.Marshal(event)
    if err!=nil {t.Fatal(err)}
    forged:=httptest.NewRecorder()
    handler.ServeHTTP(forged,httptest.NewRequest(http.MethodPost,"/v1/consultation-provider-webhooks",strings.NewReader(string(unsignedJSON))))
    if forged.Code!=http.StatusUnauthorized || fake.applied!=0 {
        t.Fatalf("unsigned provider fact accepted: status=%d applied=%d",forged.Code,fake.applied)
    }
    // Same envelope, now signed using the established canonical connector format.
    signed:=signProviderWebhook(t,event,private)
    signedJSON,err:=json.Marshal(signed)
    if err!=nil {t.Fatal(err)}
    accepted:=httptest.NewRecorder()
    handler.ServeHTTP(accepted,httptest.NewRequest(http.MethodPost,"/v1/consultation-provider-webhooks",strings.NewReader(string(signedJSON))))
    if accepted.Code!=http.StatusOK || fake.applied!=1 {
        t.Fatalf("signed provider event rejected: status=%d applied=%d body=%s",accepted.Code,fake.applied,accepted.Body.String())
    }
    noSession:=httptest.NewRecorder()
    handler.ServeHTTP(noSession,httptest.NewRequest(http.MethodGet,"/v1/consultations/00000000-0000-4000-8000-000000000001/result",nil))
    if noSession.Code!=http.StatusUnauthorized || fake.read!=0 {
        t.Fatalf("untrusted result read accepted: status=%d reads=%d body=%s",noSession.Code,fake.read,noSession.Body.String())
    }
}
