import type {
  MobileNotificationProjectionV1,
  MobileNotificationTransportV1,
} from "../../../packages/contracts/src/r2-transaction-edge";

export type NotificationClientConfig = {
  baseURL: string;
  sessionCookie?: string;
};

type FetchOptions = {
  method?: string;
  headers?: Record<string, string>;
  credentials?: "include";
};

type FetchResponse = {
  status: number;
  json(): Promise<unknown>;
};

export type NotificationFetch = (
  url: string,
  options?: FetchOptions,
) => Promise<FetchResponse>;

const defaultFetch: NotificationFetch = (url, options) => fetch(url, options);

function exactKeys(value: Record<string, unknown>, expected: readonly string[]): boolean {
  const actual = Object.keys(value).sort();
  const wanted = [...expected].sort();
  return actual.length === wanted.length && actual.every((key, index) => key === wanted[index]);
}

function canonicalBaseURL(raw: string): string {
  const candidate = raw.trim().replace(/\/$/, "");
  let parsed: URL;
  try {
    parsed = new URL(candidate);
  } catch {
    throw new Error("NOTIFICATION_BASE_URL_INVALID");
  }
  const loopback =
    parsed.protocol === "http:" &&
    (parsed.hostname === "127.0.0.1" || parsed.hostname === "localhost");
  if (
    (parsed.protocol !== "https:" && !loopback) ||
    parsed.username !== "" ||
    parsed.password !== "" ||
    parsed.search !== "" ||
    parsed.hash !== "" ||
    (parsed.pathname !== "" && parsed.pathname !== "/")
  ) {
    throw new Error("NOTIFICATION_BASE_URL_INVALID");
  }
  return candidate;
}

export function requireNotificationTransport(value: unknown): MobileNotificationTransportV1 {
  if (!value || typeof value !== "object") {
    throw new Error("NOTIFICATION_TRANSPORT_INVALID");
  }
  const candidate = value as Record<string, unknown>;
  if (
    !exactKeys(candidate, ["contract_version", "delivery_id", "intent_id"]) ||
    candidate.contract_version !== "notification-transport-v1" ||
    typeof candidate.delivery_id !== "string" ||
    !candidate.delivery_id.trim() ||
    typeof candidate.intent_id !== "string" ||
    !candidate.intent_id.trim()
  ) {
    throw new Error("NOTIFICATION_TRANSPORT_INVALID");
  }
  return candidate as MobileNotificationTransportV1;
}

export function requireNotificationProjection(value: unknown): MobileNotificationProjectionV1 {
  if (!value || typeof value !== "object") {
    throw new Error("NOTIFICATION_PROJECTION_INVALID");
  }
  const candidate = value as Record<string, unknown>;
  const validStates = new Set([
    "PENDING",
    "SENT",
    "DELIVERED",
    "FAILED_RETRYABLE",
    "SUPPRESSED",
  ]);
  if (
    !exactKeys(candidate, [
      "contract_version",
      "delivery_id",
      "intent_id",
      "purpose",
      "related_object_ref",
      "channel",
      "delivery_state",
      "data_class",
      "preview_mode",
    ]) ||
    candidate.contract_version !== "notification-projection-v1" ||
    typeof candidate.delivery_id !== "string" ||
    !candidate.delivery_id.trim() ||
    typeof candidate.intent_id !== "string" ||
    !candidate.intent_id.trim() ||
    typeof candidate.purpose !== "string" ||
    !candidate.purpose.trim() ||
    typeof candidate.related_object_ref !== "string" ||
    !candidate.related_object_ref.trim() ||
    candidate.channel !== "PUSH" ||
    typeof candidate.delivery_state !== "string" ||
    !validStates.has(candidate.delivery_state) ||
    typeof candidate.data_class !== "string" ||
    !candidate.data_class.trim() ||
    (candidate.preview_mode !== "GENERIC" && candidate.preview_mode !== "FULL")
  ) {
    throw new Error("NOTIFICATION_PROJECTION_INVALID");
  }
  return candidate as MobileNotificationProjectionV1;
}

function sensitiveDataClass(dataClass: string): boolean {
  const canonical = dataClass.trim().toUpperCase();
  return canonical !== "PUBLIC" && canonical !== "INTERNAL";
}

export async function resolvePushNotification(
  transportValue: unknown,
  config: NotificationClientConfig,
  request: NotificationFetch = defaultFetch,
): Promise<MobileNotificationProjectionV1> {
  const transport = requireNotificationTransport(transportValue);
  const baseURL = canonicalBaseURL(config.baseURL);
  const headers: Record<string, string> = {Accept: "application/json"};
  if (config.sessionCookie) {
    headers.Cookie = config.sessionCookie;
  }
  const response = await request(
    `${baseURL}/v1/mobile/notification-deliveries/${encodeURIComponent(transport.delivery_id)}`,
    {method: "GET", headers, credentials: "include"},
  );
  if (response.status !== 200) {
    throw new Error(`NOTIFICATION_HTTP_${response.status}`);
  }
  const projection = requireNotificationProjection(await response.json());
  if (
    projection.delivery_id !== transport.delivery_id ||
    projection.intent_id !== transport.intent_id
  ) {
    throw new Error("NOTIFICATION_TRANSPORT_SCOPE_MISMATCH");
  }
  if (sensitiveDataClass(projection.data_class) && projection.preview_mode !== "GENERIC") {
    throw new Error("NOTIFICATION_SENSITIVE_PREVIEW_UNSAFE");
  }
  return projection;
}
