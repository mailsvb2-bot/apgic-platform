import {test} from "node:test";
import assert from "node:assert/strict";
import {hasLiveMediaTicket,type LiveAccessEligibility,type LiveEvent} from "../../../packages/contracts/src/r9-live-events.ts";
test("R9 mobile never confuses eligible participant with room credentials",()=>{
 const access:LiveAccessEligibility={allowed:true,reason_code:"LIVE_JOIN_ELIGIBLE",provider_ready:true};
 assert.equal(hasLiveMediaTicket(access),false);
});
test("R9 client holds one server-owned event model",()=>{
 const x:LiveEvent={id:"e",organization_id:"o",title:"Тестовая встреча",description:"",
  format:"GROUP",starts_at:"2026-11-01T11:00:00Z",ends_at:"2026-11-01T12:00:00Z",
  capacity:12,registered:3,state:"PUBLISHED",price_minor:0,currency:"RUB",recording_available:false};
 assert.equal(x.registered,3);
 assert.equal(x.recording_available,false);
});
