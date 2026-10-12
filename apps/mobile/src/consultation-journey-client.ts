// R3 shared server business truth, never local proof of provider attendance.
import type { BookingClientConfig } from "./mobile-booking-client.ts";

export type ConsultationReadiness = {
  booking_id: string; role: "CLIENT" | "SPECIALIST"; booking_state: string;
  opens_at: string; closes_at: string; client_ready_at?: string; specialist_ready_at?: string;
};
export type ConsultationResult = {
  booking_id: string; state: "SCHEDULED" | "READY" | "IN_PROGRESS" | "RECOVERING" | "TECHNICAL_FAILURE" | "COMPLETED";
  provider_instance_id: string; completion_evidence_ref?: string;
};
export type ClientOutcome = {
  booking_id: string; progress: number; match_satisfaction: number; reported_at: string;
};
type Transport = (url: string, init: {
  method: string; credentials: "include"; cache: "no-store"; headers: Record<string,string>; body?: string;
}) => Promise<{ status: number; json(): Promise<unknown> }>;

function endpoint(config: BookingClientConfig, bookingID: string, endpointName: "ready" | "result" | "outcome"): string {
  if (!/^[0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}$/.test(bookingID)) {
    throw new Error("CONSULT_BOOKING_ID_INVALID");
  }
  let url: URL;
  try { url = new URL(config.baseURL); } catch { throw new Error("CONSULT_ORIGIN_INVALID"); }
  const loopback = url.protocol === "http:" && (url.hostname === "localhost" || url.hostname === "127.0.0.1");
  if ((url.protocol !== "https:" && !loopback) || url.username || url.password ||
      url.search || url.hash || (url.pathname !== "/" && url.pathname !== "")) {
    throw new Error("CONSULT_ORIGIN_INVALID");
  }
  return url.origin + "/v1/consultations/" + bookingID + "/" + endpointName;
}
function readOrThrow(status: number) {
  if (status === 401 || status === 403) throw new Error("CONSULT_SESSION_REQUIRED");
  if (status === 404) throw new Error("CONSULT_NOT_FOUND");
  if (status < 200 || status >= 300) throw new Error("CONSULT_UNAVAILABLE");
}
function transportDefault(url: string, init: Parameters<Transport>[1]) {
  return fetch(url, init);
}
async function execute(config: BookingClientConfig, bookingID: string,
  segment: "ready" | "result" | "outcome", method: "GET" | "POST",
  request: Transport = transportDefault, body?: string): Promise<unknown> {
  const response = await request(endpoint(config, bookingID, segment), {
    method, credentials: "include", cache: "no-store",
    headers: { Accept: "application/json",
      ...(config.sessionCookie ? { Cookie: config.sessionCookie } : {}),
      ...(method === "POST" ? { "X-APGIC-Request": "consultation-v1" } : {}),
      ...(body ? { "Content-Type": "application/json" } : {}) },
    ...(body ? {body} : {}),
  });
  readOrThrow(response.status);
  return response.json();
}
export async function readConsultationReadiness(config: BookingClientConfig, bookingID: string,
  request?: Transport): Promise<ConsultationReadiness> {
  const v = await execute(config, bookingID, "ready", "GET", request);
  const r = v as Partial<ConsultationReadiness> | null;
  if (!r || r.booking_id !== bookingID || (r.role !== "CLIENT" && r.role !== "SPECIALIST") ||
      !r.opens_at || !Number.isFinite(Date.parse(r.opens_at)) ||
      !r.closes_at || !Number.isFinite(Date.parse(r.closes_at))) throw new Error("CONSULT_READINESS_INVALID");
  return r as ConsultationReadiness;
}
export async function confirmConsultationReadiness(config: BookingClientConfig, bookingID: string,
  request?: Transport): Promise<ConsultationReadiness> {
  const v = await execute(config, bookingID, "ready", "POST", request);
  const r = v as Partial<ConsultationReadiness> | null;
  if (!r || r.booking_id !== bookingID || (r.role !== "CLIENT" && r.role !== "SPECIALIST")) {
    throw new Error("CONSULT_READINESS_INVALID");
  }
  return r as ConsultationReadiness;
}
export async function readProviderConsultationResult(config: BookingClientConfig, bookingID: string,
  request?: Transport): Promise<ConsultationResult> {
  const v = await execute(config, bookingID, "result", "GET", request);
  const r = v as Partial<ConsultationResult> | null;
  const known = ["SCHEDULED","READY","IN_PROGRESS","RECOVERING","TECHNICAL_FAILURE","COMPLETED"];
  if (!r || r.booking_id !== bookingID || !known.includes(r.state ?? "") || !r.provider_instance_id ||
      (r.state === "COMPLETED" && !r.completion_evidence_ref?.trim())) {
    throw new Error("CONSULT_RESULT_INVALID");
  }
  return r as ConsultationResult;
}
export async function recordClientOutcome(config: BookingClientConfig, bookingID: string, progress: number,
  satisfaction: number, idempotencyKey: string, request?: Transport): Promise<ClientOutcome> {
  if (!Number.isInteger(progress) || progress < -2 || progress > 2 ||
      !Number.isInteger(satisfaction) || satisfaction < 1 || satisfaction > 5 ||
      idempotencyKey.length < 16 || idempotencyKey.length > 128) throw new Error("CONSULT_OUTCOME_INVALID");
  const v = await execute(config,bookingID,"outcome","POST",request,
    JSON.stringify({progress,match_satisfaction:satisfaction,idempotency_key:idempotencyKey}));
  const r = v as Partial<ClientOutcome> | null;
  if (!r || r.booking_id !== bookingID || r.progress !== progress || r.match_satisfaction !== satisfaction ||
      !r.reported_at || !Number.isFinite(Date.parse(r.reported_at))) throw new Error("CONSULT_OUTCOME_INVALID");
  return r as ClientOutcome;
}

export type CommunicationJoinGrant = {
  booking_id: string; provider_instance_id: string; join_uri: string;
  credential_scope: "ROOM_JOIN"; credential_expires_at: string; reason_code: string;
};
export async function requestConsultationJoin(config: BookingClientConfig, bookingID: string,
  idempotencyKey: string, request: Transport = transportDefault): Promise<CommunicationJoinGrant> {
  if (idempotencyKey.length < 16 || idempotencyKey.length > 128 || /\s/.test(idempotencyKey)) {
    throw new Error("COMM_JOIN_KEY_INVALID");
  }
  const url = endpoint(config, bookingID, "ready").replace(/ready$/, "join");
  const response = await request(url, {
    method: "POST", credentials: "include", cache: "no-store",
    headers: {Accept:"application/json", "X-APGIC-Request":"consultation-v1",
      "Idempotency-Key":idempotencyKey,
      ...(config.sessionCookie ? {Cookie:config.sessionCookie} : {})},
  });
  readOrThrow(response.status);
  const v: unknown = await response.json();
  const g = v as Partial<CommunicationJoinGrant> | null;
  if (!g || g.booking_id!==bookingID || g.credential_scope!=="ROOM_JOIN" ||
      !g.provider_instance_id || !g.join_uri || !g.join_uri.startsWith("https://") ||
      !g.credential_expires_at || !Number.isFinite(Date.parse(g.credential_expires_at))) {
    throw new Error("COMM_JOIN_GRANT_INVALID");
  }
  const parsed = new URL(g.join_uri);
  if (parsed.protocol !== "https:" || parsed.username || parsed.password || !parsed.hostname) {
    throw new Error("COMM_JOIN_GRANT_INVALID");
  }
  return g as CommunicationJoinGrant;
}
