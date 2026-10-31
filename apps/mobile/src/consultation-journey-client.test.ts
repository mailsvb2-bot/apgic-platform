import assert from "node:assert/strict";
import test from "node:test";
import {
 readConsultationReadiness, confirmConsultationReadiness,
 readProviderConsultationResult, recordClientOutcome, requestConsultationJoin,
} from "./consultation-journey-client.ts";

const id = "00000000-0000-4000-8000-000000000001";
const cfg = {baseURL:"https://apgic.ru", sessionCookie:"__Host-apgic_session=test"};
const ready = {booking_id:id,role:"CLIENT",booking_state:"CONFIRMED",
  opens_at:"2026-10-12T10:00:00Z",closes_at:"2026-10-12T11:00:00Z"};
test("native readiness uses authenticated canonical API; never synthesizes presence",async()=>{
  let post=false;
  const t=async (_:string,init:{method:string;headers:Record<string,string>})=>{
    assert.equal(init.headers.Cookie,cfg.sessionCookie);
    if(init.method==="POST"){post=true;assert.equal(init.headers["X-APGIC-Request"],"consultation-v1");}
    return {status:200,async json(){return ready;}};
  };
  assert.equal((await readConsultationReadiness(cfg,id,t)).role,"CLIENT");
  assert.equal((await confirmConsultationReadiness(cfg,id,t)).booking_id,id);
  assert.equal(post,true);
  await assert.rejects(readConsultationReadiness({...cfg,baseURL:"http://foreign.test"},id,t),/CONSULT_ORIGIN_INVALID/);
});
test("native result requires signed-provider completion projection, not just status",async()=>{
  const t=(data:unknown,status=200)=>async()=>({status,async json(){return data;}});
  await assert.rejects(readProviderConsultationResult(cfg,id,t({booking_id:id,state:"COMPLETED",provider_instance_id:"comm-1"})),/CONSULT_RESULT_INVALID/);
  const result=await readProviderConsultationResult(cfg,id,t({booking_id:id,state:"COMPLETED",
    provider_instance_id:"comm-1",completion_evidence_ref:"evidence/provider-1"}));
  assert.equal(result.state,"COMPLETED");
  await assert.rejects(readProviderConsultationResult(cfg,id,t({},403)),/CONSULT_SESSION_REQUIRED/);
});
test("native client outcome cannot be created with malformed values or wrong booking",async()=>{
  const t=async (_:string,init:{headers:Record<string,string>;body?:string})=>{
    assert.equal(init.headers["X-APGIC-Request"],"consultation-v1");
    assert.match(init.body??"", /match_satisfaction/);
    return {status:200,async json(){return {booking_id:id,progress:1,match_satisfaction:5,reported_at:"2026-10-12T11:10:00Z"};}};
  };
  assert.equal((await recordClientOutcome(cfg,id,1,5,"client-feedback-retry-001",t)).progress,1);
  await assert.rejects(recordClientOutcome(cfg,id,20,5,"client-feedback-retry-001",t),/CONSULT_OUTCOME_INVALID/);
  await assert.rejects(recordClientOutcome(cfg,id,1,5,"too-short",t),/CONSULT_OUTCOME_INVALID/);
});

test("native room authorization uses one booked resource and rejects unsafe server grants", async () => {
  let issuerCalls = 0;
  const roomRequest = async (url: string, init: {
    method: string; headers: Record<string,string>;
  }) => {
    issuerCalls++;
    assert.equal(url, "https://apgic.ru/v1/consultations/" + id + "/join");
    assert.equal(init.headers["X-APGIC-Request"], "consultation-v1");
    assert.equal(init.headers["Idempotency-Key"], "join-credential-key-00001");
    assert.equal(init.headers.Cookie, cfg.sessionCookie);
    return { status: 200, async json() {
      return {
        booking_id: id, provider_instance_id: "provider-1",
        join_uri: "https://media.example/session?id=opaque", credential_scope: "ROOM_JOIN",
        credential_expires_at: "2030-10-12T12:00:00Z", reason_code: "COMM_JOIN_ALLOWED",
      };
    } };
  };
  const grant = await requestConsultationJoin(cfg,id,"join-credential-key-00001",roomRequest);
  assert.equal(grant.credential_scope, "ROOM_JOIN");
  assert.equal(issuerCalls,1);
  await assert.rejects(
    requestConsultationJoin(cfg,id,"invalid key with spaces",roomRequest),/COMM_JOIN_KEY_INVALID/
  );
  assert.equal(issuerCalls,1);
  await assert.rejects(requestConsultationJoin(cfg,id,"join-credential-key-00001",
    async()=>({status:200,async json(){return {...grant,join_uri:"https://media.example@evil.example/"};}})),
    /COMM_JOIN_GRANT_INVALID/);
  await assert.rejects(requestConsultationJoin(cfg,id,"join-credential-key-00001",
    async()=>({status:503,async json(){return {};}})),/CONSULT_UNAVAILABLE/);
});
