import type { MobileCheckoutMutationResponse } from "../../../packages/contracts/src/generated/apgic-v1";

export type OfflineCheckoutState =
  | "LOCAL_PENDING"
  | "SYNCING"
  | "SERVER_CONFIRMED"
  | "CONFLICT"
  | "FAILED";

export type OfflineCheckoutQueueItem = {
  contract_version: "offline-checkout-queue-v1";
  idempotency_key: string;
  hold_id: string;
  method_code: string;
  state: OfflineCheckoutState;
  attempts: number;
  max_attempts: number;
  created_at: string;
  expires_at: string;
  reason_code?: string;
  side_effect_ref?: string;
};

export type OfflineMutationStorage = {
  load(): Promise<string | null>;
  save(value: string): Promise<void>;
  clear(): Promise<void>;
};

type FetchOptions = {
  method?: string;
  headers?: Record<string, string>;
  body?: string;
  credentials?: "include";
};

type FetchResponse = {
  status: number;
  json(): Promise<unknown>;
};

export type OfflineCheckoutFetch = (
  url: string,
  options?: FetchOptions,
) => Promise<FetchResponse>;

export type OfflineCheckoutClientConfig = {
  baseURL: string;
  sessionCookie?: string;
};

const defaultFetch: OfflineCheckoutFetch = (url, options) => fetch(url, options);
const queueKeys = [
  "contract_version",
  "idempotency_key",
  "hold_id",
  "method_code",
  "state",
  "attempts",
  "max_attempts",
  "created_at",
  "expires_at",
  "reason_code",
  "side_effect_ref",
] as const;

function canonicalBaseURL(raw: string): string {
  const candidate = raw.trim().replace(/\/$/, "");
  let parsed: URL;
  try {
    parsed = new URL(candidate);
  } catch {
    throw new Error("OFFLINE_MUTATION_BASE_URL_INVALID");
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
    throw new Error("OFFLINE_MUTATION_BASE_URL_INVALID");
  }
  return candidate;
}

function exactQueueKeys(candidate: Record<string, unknown>): boolean {
  return Object.keys(candidate).every((key) =>
    (queueKeys as readonly string[]).includes(key),
  );
}

function validState(value: unknown): value is OfflineCheckoutState {
  return (
    value === "LOCAL_PENDING" ||
    value === "SYNCING" ||
    value === "SERVER_CONFIRMED" ||
    value === "CONFLICT" ||
    value === "FAILED"
  );
}

export function requireOfflineCheckoutQueueItem(
  value: unknown,
): OfflineCheckoutQueueItem {
  if (!value || typeof value !== "object") {
    throw new Error("OFFLINE_MUTATION_QUEUE_INVALID");
  }
  const candidate = value as Record<string, unknown>;
  if (
    !exactQueueKeys(candidate) ||
    candidate.contract_version !== "offline-checkout-queue-v1" ||
    typeof candidate.idempotency_key !== "string" ||
    !/^[A-Za-z0-9._:-]{1,128}$/.test(candidate.idempotency_key) ||
    typeof candidate.hold_id !== "string" ||
    !candidate.hold_id.trim() ||
    typeof candidate.method_code !== "string" ||
    !candidate.method_code.trim() ||
    !validState(candidate.state) ||
    typeof candidate.attempts !== "number" ||
    !Number.isInteger(candidate.attempts) ||
    candidate.attempts < 0 ||
    typeof candidate.max_attempts !== "number" ||
    !Number.isInteger(candidate.max_attempts) ||
    candidate.max_attempts < 1 ||
    candidate.max_attempts > 10 ||
    candidate.attempts > candidate.max_attempts ||
    typeof candidate.created_at !== "string" ||
    Number.isNaN(Date.parse(candidate.created_at)) ||
    typeof candidate.expires_at !== "string" ||
    Number.isNaN(Date.parse(candidate.expires_at)) ||
    Date.parse(candidate.expires_at) <= Date.parse(candidate.created_at) ||
    (candidate.reason_code !== undefined &&
      typeof candidate.reason_code !== "string") ||
    (candidate.side_effect_ref !== undefined &&
      typeof candidate.side_effect_ref !== "string")
  ) {
    throw new Error("OFFLINE_MUTATION_QUEUE_INVALID");
  }
  return candidate as OfflineCheckoutQueueItem;
}

export function createOfflineCheckoutQueueItem(input: {
  idempotencyKey: string;
  holdID: string;
  methodCode: string;
  now: Date;
  expiresAt: Date;
  maxAttempts?: number;
}): OfflineCheckoutQueueItem {
  return requireOfflineCheckoutQueueItem({
    contract_version: "offline-checkout-queue-v1",
    idempotency_key: input.idempotencyKey,
    hold_id: input.holdID,
    method_code: input.methodCode,
    state: "LOCAL_PENDING",
    attempts: 0,
    max_attempts: input.maxAttempts ?? 3,
    created_at: input.now.toISOString(),
    expires_at: input.expiresAt.toISOString(),
  });
}

export async function persistOfflineCheckout(
  storage: OfflineMutationStorage,
  item: OfflineCheckoutQueueItem,
): Promise<void> {
  await storage.save(JSON.stringify(requireOfflineCheckoutQueueItem(item)));
}

export async function loadOfflineCheckout(
  storage: OfflineMutationStorage,
): Promise<OfflineCheckoutQueueItem | null> {
  const raw = await storage.load();
  if (raw === null) {
    return null;
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    await storage.clear();
    throw new Error("OFFLINE_MUTATION_QUEUE_CORRUPT");
  }
  try {
    return requireOfflineCheckoutQueueItem(parsed);
  } catch (error) {
    await storage.clear();
    throw error;
  }
}

function parseCheckoutResponse(value: unknown): MobileCheckoutMutationResponse {
  if (!value || typeof value !== "object") {
    throw new Error("OFFLINE_MUTATION_RESPONSE_INVALID");
  }
  const candidate = value as Partial<MobileCheckoutMutationResponse>;
  if (
    candidate.contract_version !== "offline-checkout-mutation-v1" ||
    (candidate.outcome !== "APPLIED" &&
      candidate.outcome !== "DUPLICATE_APPLIED") ||
    typeof candidate.reason_code !== "string" ||
    !candidate.reason_code ||
    typeof candidate.side_effect_ref !== "string" ||
    !candidate.side_effect_ref.startsWith("checkout/") ||
    !candidate.checkout ||
    typeof candidate.checkout !== "object" ||
    typeof candidate.checkout.id !== "string" ||
    candidate.side_effect_ref !== `checkout/${candidate.checkout.id}`
  ) {
    throw new Error("OFFLINE_MUTATION_RESPONSE_INVALID");
  }
  return candidate as MobileCheckoutMutationResponse;
}

async function errorCode(response: FetchResponse): Promise<string> {
  try {
    const value = await response.json();
    if (
      value &&
      typeof value === "object" &&
      typeof (value as {code?: unknown}).code === "string"
    ) {
      return (value as {code: string}).code;
    }
  } catch {
    // Invalid error payload is mapped to a stable local failure below.
  }
  return `OFFLINE_MUTATION_HTTP_${response.status}`;
}

export async function syncOfflineCheckout(
  item: OfflineCheckoutQueueItem,
  storage: OfflineMutationStorage,
  config: OfflineCheckoutClientConfig,
  request: OfflineCheckoutFetch = defaultFetch,
  now: Date = new Date(),
): Promise<OfflineCheckoutQueueItem> {
  let current = requireOfflineCheckoutQueueItem(item);
  if (
    current.state === "SERVER_CONFIRMED" ||
    current.state === "CONFLICT" ||
    current.state === "FAILED"
  ) {
    return current;
  }
  if (now.getTime() >= Date.parse(current.expires_at)) {
    current = {
      ...current,
      state: "FAILED",
      reason_code: "MUTATION_QUEUE_EXPIRED",
    };
    await persistOfflineCheckout(storage, current);
    return current;
  }
  if (current.attempts >= current.max_attempts) {
    current = {
      ...current,
      state: "FAILED",
      reason_code: "MUTATION_RETRY_EXHAUSTED",
    };
    await persistOfflineCheckout(storage, current);
    return current;
  }

  current = {
    ...current,
    state: "SYNCING",
    attempts: current.attempts + 1,
    reason_code: undefined,
  };
  await persistOfflineCheckout(storage, current);

  const headers: Record<string, string> = {
    Accept: "application/json",
    "Content-Type": "application/json",
    "Idempotency-Key": current.idempotency_key,
  };
  if (config.sessionCookie) {
    headers.Cookie = config.sessionCookie;
  }

  let response: FetchResponse;
  try {
    response = await request(
      canonicalBaseURL(config.baseURL) + "/v1/mobile/checkout-instructions",
      {
        method: "POST",
        headers,
        credentials: "include",
        body: JSON.stringify({
          hold_id: current.hold_id,
          method_code: current.method_code,
        }),
      },
    );
  } catch {
    current = {
      ...current,
      state: "LOCAL_PENDING",
      reason_code: "MUTATION_RETRY_PENDING",
    };
    await persistOfflineCheckout(storage, current);
    return current;
  }

  if (response.status === 200 || response.status === 201) {
    const result = parseCheckoutResponse(await response.json());
    current = {
      ...current,
      state: "SERVER_CONFIRMED",
      reason_code: result.reason_code,
      side_effect_ref: result.side_effect_ref,
    };
    await storage.clear();
    return current;
  }

  const code = await errorCode(response);
  if (response.status === 409 && code === "MUTATION_IDEMPOTENCY_CONFLICT") {
    current = {...current, state: "CONFLICT", reason_code: code};
    await persistOfflineCheckout(storage, current);
    return current;
  }
  if (response.status >= 500) {
    current = {
      ...current,
      state: "LOCAL_PENDING",
      reason_code: "MUTATION_RETRY_PENDING",
    };
    await persistOfflineCheckout(storage, current);
    return current;
  }

  current = {...current, state: "FAILED", reason_code: code};
  await persistOfflineCheckout(storage, current);
  return current;
}
