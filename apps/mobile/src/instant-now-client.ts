import type {MobileDemandClientOptions} from "./mobile-demand-client.ts";

export type InstantDispatchState = "UNFILLED" | "OFFERED" | "ACCEPTED" | "DECLINED" | "EXPIRED" | "CANCELLED" | "INVALIDATED";

export type InstantDispatch = {
  id: string;
  request_key: string;
  client_identity_id: string;
  help_intent_id: string;
  specialist_identity_id?: string;
  offer_id?: string;
  topic_id?: string;
  price_minor?: number;
  currency?: string;
  state: InstantDispatchState;
  next_action: "WAIT_SPECIALIST_RESPONSE" | "PAY_REAL_PROVIDER_REQUIRED" | "RETRY_OR_BOOK_LATER";
  room_ready: false;
  expires_at?: string;
};

export type InstantSpecialistDispatch = Pick<
  InstantDispatch,
  "id" | "state" | "topic_id" | "price_minor" | "currency" | "expires_at" | "next_action" | "room_ready"
>;

function verifySpecialistDispatch(d:InstantSpecialistDispatch):InstantSpecialistDispatch {
 if(!d.id || d.room_ready!==false || !["OFFERED","ACCEPTED","DECLINED","EXPIRED"].includes(d.state)) {
  throw new Error("DUTY_SPECIALIST_CONTRACT_INVALID");
 }
 // Specialist projections must NEVER include raw request keys or client IDs.
 if("client_identity_id" in d || "help_intent_id" in d || "request_key" in d) {
  throw new Error("DUTY_SPECIALIST_PRIVACY_LEAK");
 }
 return d;
}

export type InstantPresence = {
  specialist_identity_id: string;
  offer_id?: string;
  state: "OFFLINE" | "AVAILABLE_NOW" | "OFFERED" | "RESERVED" | "IN_CONSULTATION" | "COOLDOWN";
  available_until?: string;
  version: number;
};

export type InstantSpecialistOffer = {
  id: string;
  product_id: string;
  topic_id: string;
  price_minor: number;
  currency: string;
  state: string;
};

const origin = (raw: string) => raw.replace(/\/$/, "");

async function instantFetch<T>(
  options: MobileDemandClientOptions, path: string, method: string = "GET", body?: unknown,
): Promise<T> {
  const headers: Record<string,string> = {"content-type": "application/json"};
  if (options.sessionCookie) headers.cookie=options.sessionCookie;
  const response = await fetch(origin(options.baseURL)+"/v1"+path,{
    method, headers, credentials:"include", cache:"no-store",
    body: body===undefined?undefined:JSON.stringify(body),
  });
  const json = await response.json() as T & {code?:string;message_safe?:string};
  if(!response.ok) throw new Error(json.message_safe || json.code || "DUTY_UNAVAILABLE");
  return json;
}

function verifyDispatch(d: InstantDispatch): InstantDispatch {
  if (!d.id || !d.request_key || d.room_ready!==false ||
    !["UNFILLED","OFFERED","ACCEPTED","DECLINED","EXPIRED","CANCELLED","INVALIDATED"].includes(d.state)) {
    throw new Error("DUTY_INVALID_PROVIDER_CONTRACT");
  }
  if(d.state==="ACCEPTED" && d.next_action!=="PAY_REAL_PROVIDER_REQUIRED")
    throw new Error("DUTY_UNPAID_ROOM_DENIED");
  return d;
}

export async function createInstantDispatch(intentID:string,options:MobileDemandClientOptions):Promise<InstantDispatch> {
  // The server owns the idempotency key so crypto/randomUUID availability is
  // never a hidden iOS/Android dependency.
  const key = await instantFetch<{request_key:string}>(options,"/instant/request-key");
  if(!key.request_key) throw new Error("DUTY_REQUEST_KEY_REQUIRED");
  try {
    return verifyDispatch(await instantFetch<InstantDispatch>(options,"/instant/dispatches","POST",{
      help_intent_id:intentID,request_key:key.request_key,
    }));
  } catch (originalError) {
    // Request may have committed before the mobile connection was interrupted.
    // Recover only this signed owner's confirmed intent, never synthesize success.
    try {
      const recovered=await readActiveInstantDispatch(options);
      if(recovered.help_intent_id===intentID && recovered.room_ready===false) return recovered;
    } catch {
      // No active dispatch to recover; surface the original failure.
    }
    throw originalError;
  }
}

export async function readActiveInstantDispatch(options:MobileDemandClientOptions):Promise<InstantDispatch> {
  return verifyDispatch(await instantFetch<InstantDispatch>(options,"/instant/active"));
}

export async function readInstantDispatch(id:string,options:MobileDemandClientOptions):Promise<InstantDispatch> {
  return verifyDispatch(await instantFetch<InstantDispatch>(options,
    "/instant/dispatches/"+encodeURIComponent(id)));
}

export async function readInstantPresence(options:MobileDemandClientOptions):Promise<InstantPresence> {
  return instantFetch<InstantPresence>(options,"/instant/presence");
}

export async function updateInstantPresence(
  offerID:string,available:boolean,options:MobileDemandClientOptions,
):Promise<InstantPresence> {
  return instantFetch<InstantPresence>(options,"/instant/presence","PUT",{
    offer_id:available?offerID:"",available,
  });
}

export async function listInstantSpecialistOffers(options:MobileDemandClientOptions):Promise<InstantSpecialistOffer[]> {
  const r=await instantFetch<{offers:InstantSpecialistOffer[]}>(options,"/specialist/offers");
  return r.offers ?? [];
}

export async function listInstantInbox(options:MobileDemandClientOptions):Promise<InstantSpecialistDispatch[]> {
  const r=await instantFetch<{dispatches:InstantSpecialistDispatch[]}>(options,"/instant/inbox");
  return (r.dispatches || []).map(verifySpecialistDispatch);
}

export async function answerInstantDispatch(
  id:string,accept:boolean,options:MobileDemandClientOptions,
):Promise<InstantSpecialistDispatch> {
  return verifySpecialistDispatch(await instantFetch<InstantSpecialistDispatch>(options,
    "/instant/dispatches/"+encodeURIComponent(id)+"/answer","POST",{accept}));
}

export async function cancelInstantDispatch(id:string, options:MobileDemandClientOptions):Promise<InstantDispatch> {
  const d=verifyDispatch(await instantFetch<InstantDispatch>(options,
    "/instant/dispatches/"+encodeURIComponent(id)+"/cancel","POST",{}));
  if(d.state!=="CANCELLED")throw new Error("DUTY_CANCEL_UNCONFIRMED");
  return d;
}

export type InstantFulfillment = {
  dispatch_id: string;
  dispatch_state: InstantDispatchState;
  booking_id?: string;
  booking_state?: string;
  payment_verified: boolean;
  next_action: "AWAIT_CANONICAL_R2_BOOKING" | "AWAIT_VERIFIED_PROVIDER_PAYMENT" | "USE_R3_BOOKING_ACCESS_GATE" | "RETRY_OR_BOOK_LATER";
  room_ready: false;
};

export async function readInstantFulfillment(id:string,options:MobileDemandClientOptions):Promise<InstantFulfillment> {
  const proof = await instantFetch<InstantFulfillment>(options,
    "/instant/dispatches/"+encodeURIComponent(id)+"/fulfillment");
  if(proof.dispatch_id!==id || proof.room_ready!==false ||
     (proof.payment_verified && (!proof.booking_id || proof.next_action!=="USE_R3_BOOKING_ACCESS_GATE")) ||
     (!proof.booking_id && proof.payment_verified)) {
    throw new Error("DUTY_FULFILLMENT_CONTRACT_INVALID");
  }
  return proof;
}
