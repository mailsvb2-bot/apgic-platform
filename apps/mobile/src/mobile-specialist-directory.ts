import type {
 PublicSpecialist,
 PublicBookableOfferList,
 PublicSpecialistDirectory as SpecialistDirectoryResponse,
} from "../../../packages/contracts/src/generated/apgic-v1.ts";
export type {PublicSpecialist,SpecialistDirectoryResponse};

function isProfile(value:unknown):value is PublicSpecialist{
 if(!value||typeof value!=="object")return false;
 const p=value as Partial<PublicSpecialist>;
 return typeof p.id==="string"&&typeof p.display_name==="string"&&typeof p.profession_code==="string"&&
  typeof p.headline==="string"&&typeof p.about==="string"&&typeof p.experience_years==="number"&&
  Array.isArray(p.published_topics)&&p.published_topics.length>0&&
  p.published_topics.every(topic=>typeof topic==="string")&&
  Array.isArray(p.languages)&&Array.isArray(p.limitations) &&
  !("identity_id" in p)&&!("evidence_refs" in p)&&!("reviewer_ref" in p);
}
export async function fetchNativeSpecialistDirectory(baseURL:string,topic:string,signal?:AbortSignal):Promise<SpecialistDirectoryResponse>{
 const origin=baseURL.replace(/\/$/,"");
 const params=new URLSearchParams({limit:"10",offset:"0"});
 if(topic)params.set("topic",topic);
 const response=await fetch(`${origin}/v1/specialists?${params.toString()}`,{
  method:"GET",headers:{accept:"application/json"},signal,cache:"no-store"});
 if(!response.ok)throw new Error(response.status===503?"SPECIALIST_DIRECTORY_UNAVAILABLE":"SPECIALIST_DIRECTORY_REJECTED");
 const raw:unknown=await response.json();
 if(!raw||typeof raw!=="object")throw new Error("SPECIALIST_DIRECTORY_INVALID");
 const directory=raw as Partial<SpecialistDirectoryResponse>;
 if(!Array.isArray(directory.profiles)||!directory.profiles.every(isProfile)||
    directory.policy_version!=="qualification-v1"||directory.sort!=="DISPLAY_NAME_ASC")throw new Error("SPECIALIST_DIRECTORY_INVALID");
 return directory as SpecialistDirectoryResponse;
}
export async function fetchNativePublicSpecialist(baseURL:string,id:string,signal?:AbortSignal):Promise<PublicSpecialist|null>{
 const response=await fetch(`${baseURL.replace(/\/$/,"")}/v1/specialists/${encodeURIComponent(id)}`,{
  method:"GET",headers:{accept:"application/json"},signal,cache:"no-store"});
 if(response.status===404)return null;
 if(!response.ok)throw new Error("SPECIALIST_PROFILE_UNAVAILABLE");
 const raw:unknown=await response.json();
 if(!isProfile(raw)||raw.id!==id)throw new Error("SPECIALIST_PROFILE_INVALID");
 return raw;
}

export async function fetchNativePublicOffers(baseURL:string,id:string,signal?:AbortSignal):Promise<PublicBookableOfferList|null>{
 const response=await fetch(`${baseURL.replace(/\/$/,"")}/v1/specialists/${encodeURIComponent(id)}/offers`,{
  method:"GET",headers:{accept:"application/json"},signal,cache:"no-store"});
 if(response.status===404)return null;
 if(!response.ok)throw new Error("SPECIALIST_OFFERS_UNAVAILABLE");
 const raw:unknown=await response.json();
 if(!raw || typeof raw!=="object")throw new Error("SPECIALIST_OFFERS_INVALID");
 const list=raw as Partial<PublicBookableOfferList>;
 if(list.specialist_id!==id || list.availability_rule!=="CURRENT_ELIGIBLE_UNHELD_SLOTS"||
    !Array.isArray(list.offers) || list.offers.some(offer=>!offer||!offer.id||
     offer.currency!=="RUB"||!Number.isSafeInteger(offer.price_minor) ||offer.price_minor<=0||
     !Array.isArray(offer.slots)||offer.slots.length===0||
     offer.slots.some(slot=>!slot.slot_id|| !Number.isFinite(Date.parse(slot.starts_at))||
      !Number.isFinite(Date.parse(slot.ends_at))||Date.parse(slot.ends_at)<=Date.parse(slot.starts_at)))) {
  throw new Error("SPECIALIST_OFFERS_INVALID");
 }
 return list as PublicBookableOfferList;
}
