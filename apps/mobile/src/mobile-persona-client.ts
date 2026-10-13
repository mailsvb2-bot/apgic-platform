// One owner-scoped APGIC persona contract on Web, iOS and Android.
// Never cache transcripts, media or provider credentials on the device.
export type PersonaCard={id:string;label:string;language:string;supports_video:boolean;supports_voice:boolean;supports_text:boolean};
export type PersonaCatalog={available:boolean;personas:PersonaCard[];reason_code?:string};
export type PersonaMode="VIDEO"|"VOICE"|"TEXT";
export type PersonaSession={session_id:string;state:"PENDING"|"ACTIVE"|"FAILED"|"ENDED";mode:PersonaMode;safety_state:string};
export type PersonaStart={persona_id:string;session_purpose:"LISTEN"|"ORGANIZE_THOUGHTS"|"PREPARE_HUMAN"|"WAIT_FOR_HUMAN"|"NAVIGATE";mode:PersonaMode;language:string;style:"CALM"|"NEUTRAL"|"WARM"|"BUSINESS";privacy_mode:"PRIVATE_EPHEMERAL"};
type Fetcher=(url:string,init?:{method?:string;credentials?:"include";cache?:"no-store";headers?:Record<string,string>;body?:string})=>Promise<{ok:boolean;status:number;json():Promise<unknown>}>;
export function personaOrigin(base:string):string{
 let u:URL;try{u=new URL(base)}catch{throw Error("PERSONA_ORIGIN_INVALID")}
 const local=u.protocol==="http:"&&(u.hostname==="localhost"||u.hostname==="127.0.0.1");
 if((u.protocol!=="https:"&&!local)||u.username||u.password||u.pathname!=="/"||u.search||u.hash)throw Error("PERSONA_ORIGIN_INVALID");
 return u.origin;
}
export function safePersonaLaunchURL(raw:string):boolean{
 try{const u=new URL(raw);return u.protocol==="https:"&&!u.username&&!u.password&&!u.hash&&!!u.hostname}catch{return false}
}
async function requireSuccess(r:{ok:boolean;status:number;json():Promise<unknown>}):Promise<unknown>{
 const result=await r.json();
 if(!r.ok){const reason=result as {code?:string};throw Error(reason?.code||"PERSONA_PROVIDER_UNAVAILABLE")}
 return result;
}
export async function listNativePersonas(base:string,request:Fetcher=fetch):Promise<PersonaCatalog>{
 const result=await requireSuccess(await request(personaOrigin(base)+"/v1/personas",{credentials:"include",cache:"no-store"})) as PersonaCatalog;
 if(!Array.isArray(result.personas)||typeof result.available!=="boolean")throw Error("PERSONA_CATALOG_INVALID");
 return result;
}
export async function openNativePersona(base:string,input:PersonaStart,key:string,request:Fetcher=fetch):Promise<{session:PersonaSession;transport:{launch_url:string;mode:PersonaMode;expires_at:string}}>{
 if(input.privacy_mode!=="PRIVATE_EPHEMERAL"||key.length<8)throw Error("PERSONA_REQUEST_INVALID");
 const response=await requireSuccess(await request(personaOrigin(base)+"/v1/avatar-sessions",{
  method:"POST",credentials:"include",cache:"no-store",headers:{"Content-Type":"application/json","Idempotency-Key":key},body:JSON.stringify(input)
 })) as {session:PersonaSession;transport:{launch_url:string;mode:PersonaMode;expires_at:string}};
 if(response.session?.state!=="ACTIVE"||!response.transport||(!response.transport.launch_url&&response.transport.mode==="VIDEO")||(response.transport.launch_url&&!safePersonaLaunchURL(response.transport.launch_url)))throw Error("PERSONA_TRANSPORT_INVALID");
 return response;
}
export async function actNativePersona(base:string,sessionID:string,action:"END"|"HUMAN_HANDOFF"|"SAFETY_ESCALATE",request:Fetcher=fetch):Promise<{session:PersonaSession;provider_close?:string;handoff?:{next_action:string}}>{
 if(!sessionID||sessionID.includes("/")||sessionID.includes("?")||sessionID.includes("#"))throw Error("PERSONA_SESSION_ID_INVALID");
 return await requireSuccess(await request(personaOrigin(base)+"/v1/avatar-sessions/"+encodeURIComponent(sessionID)+"/actions",{
  method:"POST",credentials:"include",cache:"no-store",headers:{"Content-Type":"application/json"},body:JSON.stringify({action})
 })) as {session:PersonaSession;provider_close?:string;handoff?:{next_action:string}};
}

// Real provider-generated turn. Ephemeral input/output must not be cached on device.
export async function sendNativePersona(base:string,sessionID:string,message:string,request:Fetcher=fetch):Promise<string>{
 if(!/^[a-fA-F0-9-]{36}$/.test(sessionID)||!message.trim()||message.length>2000)throw Error("PERSONA_REQUEST_INVALID");
 const result=await requireSuccess(await request(personaOrigin(base)+"/v1/avatar-sessions/"+encodeURIComponent(sessionID)+"/turns",{
  method:"POST",credentials:"include",cache:"no-store",headers:{"Content-Type":"application/json"},body:JSON.stringify({message})
 })) as {reply?:unknown;source?:string};
 if(result.source!=="MODEL_GENERATED"||typeof result.reply!=="string"||!result.reply.trim())throw Error("PERSONA_MODEL_UNAVAILABLE");
 return result.reply;
}

// Resumption uses the HttpOnly/APGIC-owned client session, not local transcript
// storage or provider references. 401 is an ordinary first-use/no-cookie case.
export async function readNativeActivePersona(base:string,request:Fetcher=fetch):Promise<PersonaSession|null>{
 const response=await request(personaOrigin(base)+"/v1/avatar-sessions/current",{
  credentials:"include",cache:"no-store"
 });
 if(response.status===401)return null;
 const result=await requireSuccess(response) as {session?:PersonaSession|null};
 if(!result.session)return null;
 if(result.session.state!=="ACTIVE"||!/^[a-fA-F0-9-]{36}$/.test(result.session.session_id)){
  throw Error("PERSONA_RESUME_INVALID");
 }
 return result.session;
}
