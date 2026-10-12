import assert from "node:assert/strict";
import test from "node:test";
import {
 readConsultationReadiness, confirmConsultationReadiness,
 readProviderConsultationResult, recordClientOutcome,
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
    assert.match(init.body??"","match_satisfaction");
    return {status:200,async json(){return {booking_id:id,progress:1,match_satisfaction:5,reported_at:"2026-10-12T11:10:00Z"};}};
  };
  assert.equal((await recordClientOutcome(cfg,id,1,5,"client-feedback-retry-001",t)).progress,1);
  await assert.rejects(recordClientOutcome(cfg,id,20,5,"client-feedback-retry-001",t),/CONSULT_OUTCOME_INVALID/);
  await assert.rejects(recordClientOutcome(cfg,id,1,5,"too-short",t),/CONSULT_OUTCOME_INVALID/);
});
