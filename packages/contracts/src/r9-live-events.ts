/** R9 candidate contract: core owns registration; connector owns SFU media. */
export type LiveEventState = "DRAFT"|"PUBLISHED"|"LIVE"|"COMPLETED"|"CANCELLED";
export type LiveEventFormat = "WEBINAR"|"GROUP"|"WORKSHOP"|"QA"|"CONFERENCE";
export type LiveEvent = {
 id:string;organization_id:string;title:string;description:string;
 format:LiveEventFormat;starts_at:string;ends_at:string;capacity:number;
 registered:number;state:LiveEventState;price_minor:number;currency:string;
 recording_available:boolean;
};
export type LiveRegistration = {
 id:string;event_id:string;identity_id:string;state:"REGISTERED"|"CANCELLED";registered_at:string;
};
export type LiveAccessEligibility = {
 allowed:boolean;reason_code:string;provider_ready:boolean;
};
/** Eligibility is NOT a join token; media credentials require certified provider execution. */
export function hasLiveMediaTicket(value:LiveAccessEligibility):false {
 void value;
 return false;
}

/** Distinct signed JOINED participants, never content or recording truth. */
export type LiveAttendanceSummary = {
 event_id:string;
 registered:number;
 joined_participants:number;
 media_ready_observations:number;
 total_connected_seconds:number;
 provider_join_facts:number;
 provider_leave_facts:number;
 verified_evidence:boolean;
};
