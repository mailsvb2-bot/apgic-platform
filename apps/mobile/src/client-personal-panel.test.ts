import assert from "node:assert/strict";
import test from "node:test";
import {parseClientPersonalView} from "../../../packages/contracts/src/client-personal.ts";
test("mobile rejects incomplete personal history and does not infer consultation completion",()=>{
  const bad={identity_id:"123e4567-e89b-42d3-a456-426614174000",as_of:"2026-10-12T12:00:00Z",preferences:{display_name:"",locale:"ru",time_zone:"UTC"},
    upcoming:[],history:[{booking_id:"123e4567-e89b-42d3-a456-426614174001",state:"COMPLETED"}],requests:[]};
  assert.throws(()=>parseClientPersonalView(bad),/PERSONAL_RESPONSE_INVALID/);
});
