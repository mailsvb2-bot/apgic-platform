import assert from "node:assert/strict";
import test from "node:test";
import {parseClientPersonalView} from "../../../packages/contracts/src/client-personal.ts";

const view={
  identity_id:"123e4567-e89b-42d3-a456-426614174000",
  as_of:"2026-10-12T09:00:00Z",
  preferences:{display_name:"Анна",locale:"ru",time_zone:"Europe/Moscow"},
  upcoming:[{booking_id:"123e4567-e89b-42d3-a456-426614174001",state:"CONFIRMED",starts_at:"2026-10-15T09:00:00Z",ends_at:"2026-10-15T10:00:00Z",specialist_name:"Специалист",specialist_profession:"PSYCHOLOGIST"}],
  history:[],requests:[],
};
test("web client personal accepts canonical owner view without raw content",()=>{
  const parsed=parseClientPersonalView(view);
  assert.equal(parsed.upcoming[0].state,"CONFIRMED");
  assert.equal("transcript" in parsed,false);
});
test("web rejects forged consultation completion states and invalid timestamps",()=>{
  assert.throws(()=>parseClientPersonalView({...view,upcoming:[{...view.upcoming[0],consultation_state:"FAKE_COMPLETED"}]}),/PERSONAL_RESPONSE_INVALID/);
  assert.throws(()=>parseClientPersonalView({...view,upcoming:[{...view.upcoming[0],starts_at:"yesterday"}]}),/PERSONAL_RESPONSE_INVALID/);
});
