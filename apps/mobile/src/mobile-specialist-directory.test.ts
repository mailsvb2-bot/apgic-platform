import assert from "node:assert/strict";
import test from "node:test";
import {fetchNativePublicOffers,fetchNativePublicSpecialist,fetchNativeSpecialistDirectory} from "./mobile-specialist-directory.ts";
const profile={id:"sp-1",display_name:"Анна",profession_code:"PSYCHOLOGIST",headline:"",
 about:"",experience_years:0,languages:["ru"],limitations:[],published_topics:["anxiety"]};
function reply(body:unknown,status=200){return new Response(JSON.stringify(body),{status,headers:{"content-type":"application/json"}})}
test("native directory reads public verified server projection, never local claims",async()=>{
 const previous=globalThis.fetch;
 globalThis.fetch=async input=>{
  assert.match(String(input),/\/v1\/specialists\?limit=10&offset=0&topic=anxiety/);
  return reply({profiles:[profile],topic:"anxiety",limit:10,offset:0,has_more:false,policy_version:"qualification-v1",sort:"DISPLAY_NAME_ASC"});
 };
 try{
  const directory=await fetchNativeSpecialistDirectory("https://apgic.ru","anxiety");
  assert.equal(directory.profiles[0].display_name,"Анна");
 }finally{globalThis.fetch=previous}
});
test("native never accepts leaking evidence or unreviewed identity in public card",async()=>{
 const previous=globalThis.fetch;
 globalThis.fetch=async()=>reply({profiles:[{...profile,evidence_refs:["private"]}],topic:"anxiety",policy_version:"qualification-v1",sort:"DISPLAY_NAME_ASC"});
 try{await assert.rejects(fetchNativeSpecialistDirectory("https://apgic.ru","anxiety"),/DIRECTORY_INVALID/)}
 finally{globalThis.fetch=previous}
});
test("native removed publication returns no profile",async()=>{
 const previous=globalThis.fetch;
 globalThis.fetch=async()=>reply({code:"SPECIALIST_NOT_PUBLIC"},404);
 try{assert.equal(await fetchNativePublicSpecialist("https://apgic.ru","sp-1"),null)}
 finally{globalThis.fetch=previous}
});

test("native reads only current PostgreSQL-backed prices and unheld public slots",async()=>{
 const old=globalThis.fetch;
 globalThis.fetch=async()=>reply({
  specialist_id:"sp-1",availability_rule:"CURRENT_ELIGIBLE_UNHELD_SLOTS",
  offers:[{id:"offer-1",topic_id:"anxiety",currency:"RUB",price_minor:250000,
   slots:[{slot_id:"slot-1",starts_at:"2026-12-14T11:00:00Z",ends_at:"2026-12-14T12:00:00Z"}]}]});
 try{
  const result=await fetchNativePublicOffers("https://example.invalid","sp-1");
  assert.equal(result?.offers[0].price_minor,250000);
  assert.equal(result?.offers[0].slots.length,1);
 }finally{globalThis.fetch=old}
});
test("native public offers fail closed on fabricated or unauthorized pricing",async()=>{
 const old=globalThis.fetch;
 globalThis.fetch=async()=>reply({
  specialist_id:"sp-1",availability_rule:"CURRENT_ELIGIBLE_UNHELD_SLOTS",
  offers:[{id:"offer-1",topic_id:"anxiety",currency:"RUB",price_minor:0,slots:[]}]});
 try{
  await assert.rejects(fetchNativePublicOffers("https://example.invalid","sp-1"),/SPECIALIST_OFFERS_INVALID/);
 }finally{globalThis.fetch=old}
});
