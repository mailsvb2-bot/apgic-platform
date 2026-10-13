import test from "node:test";
import assert from "node:assert/strict";
import {personaOrigin,safePersonaLaunchURL,openNativePersona,actNativePersona,sendNativePersona,readNativeActivePersona} from "./mobile-persona-client.ts";
test("persona mobile egress always uses approved origin syntax",()=>{
 for(const url of ["http://example.com/","https://u:p@example.com/","https://example.com/extra","https://example.com/?x=1"])assert.throws(()=>personaOrigin(url));
 assert.equal(personaOrigin("https://apgic.ru/"),"https://apgic.ru");
 assert.equal(safePersonaLaunchURL("javascript:alert(1)"),false);
 assert.equal(safePersonaLaunchURL("https://runtime.example/#frag"),false);
});
test("native private session and human handoff use canonical API",async()=>{
 const requests:string[]=[];
 const request=async(url:string,init?:{body?:string})=>{
  requests.push(url+" "+(init?.body||""));
  if(url.endsWith("/actions"))return {ok:true,status:200,json:async()=>({session:{session_id:"s",state:"ACTIVE",mode:"TEXT",safety_state:"NORMAL"},handoff:{next_action:"NEW_HELP_INTENT_REQUIRED"}})};
  return {ok:true,status:201,json:async()=>({session:{session_id:"s",state:"ACTIVE",mode:"TEXT",safety_state:"NORMAL"},transport:{launch_url:"https://runtime.example/start",mode:"TEXT",expires_at:"2027-01-01T00:00:00Z"}})};
 };
 const started=await openNativePersona("https://apgic.ru/",{persona_id:"p",session_purpose:"LISTEN",mode:"TEXT",language:"ru",style:"CALM",privacy_mode:"PRIVATE_EPHEMERAL"},"native-123456",request);
 assert.equal(started.session.state,"ACTIVE");
 const result=await actNativePersona("https://apgic.ru/","s","HUMAN_HANDOFF",request);
 assert.equal(result.handoff?.next_action,"NEW_HELP_INTENT_REQUIRED");
 assert.equal(requests.some(r=>r.includes("transcript")),false);
});

test("inline private text conversation sends a real provider turn and refuses fabricated content",async()=>{
 const seen:string[]=[];
 const client=async(url:string,init?:{body?:string})=>{
  seen.push(url+" "+(init?.body||""));
  if(url.endsWith("/turns"))return {ok:true,status:200,json:async()=>({source:"MODEL_GENERATED",reply:"Ответ действующей модели"})};
  return {ok:true,status:201,json:async()=>({session:{session_id:"00000000-0000-0000-0000-000000000301",state:"ACTIVE",mode:"TEXT",safety_state:"NORMAL"},transport:{launch_url:"",mode:"TEXT",expires_at:"2027-01-01T00:00:00Z"}})};
 };
 const open=await openNativePersona("https://apgic.ru/",{persona_id:"00000000-0000-0000-0000-000000000201",session_purpose:"LISTEN",mode:"TEXT",language:"ru",style:"CALM",privacy_mode:"PRIVATE_EPHEMERAL"},"native-123456",client);
 assert.equal(open.transport.launch_url,"");
 const out=await sendNativePersona("https://apgic.ru/",open.session.session_id,"Привет",client);
 assert.equal(out,"Ответ действующей модели");
 assert.ok(seen.some(q=>q.includes("/turns")&&q.includes("Привет")));
 await assert.rejects(()=>sendNativePersona("https://apgic.ru/",open.session.session_id,"Привет",async()=>({ok:true,status:200,json:async()=>({source:"MOCK",reply:"Заглушка"})})));
});

test("native restart resumes only authenticated active session, never an old transcript", async()=>{
 const uid="00000000-0000-0000-0000-000000000301";
 const requests:string[]=[];
 const client=async(url:string)=>{
  requests.push(url);
  return {ok:true,status:200,json:async()=>({session:{session_id:uid,state:"ACTIVE",mode:"TEXT",safety_state:"NORMAL"}})};
 };
 const current=await readNativeActivePersona("https://apgic.ru/",client);
 assert.equal(current?.session_id,uid);
 assert.deepEqual(requests,["https://apgic.ru/v1/avatar-sessions/current"]);
 assert.equal("transcript" in (current||{}),false);
 const fresh=await readNativeActivePersona("https://apgic.ru/",async()=>({ok:false,status:401,json:async()=>({code:"CLIENT_SESSION_REQUIRED"})}));
 assert.equal(fresh,null);
 await assert.rejects(()=>readNativeActivePersona("https://apgic.ru/",async()=>({ok:false,status:503,json:async()=>({code:"PERSONA_PROVIDER_UNAVAILABLE"})})));
});
