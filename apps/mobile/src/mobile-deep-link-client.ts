import type { DeepLinkResolution } from "../../../packages/contracts/src/generated/apgic-v1";
import { resolveNativeDeepLink } from "./r1-cross-surface.ts";

export const canonicalAPGICOrigin = "https://apgic.ru";
const maxDeepLinkTokenLength = 4096;
const maxWorkspaceIDLength = 256;
const canonicalWorkspaceIDPattern = /^(?:client|specialist|organization):[A-Za-z0-9._~-]+$/;
const canonicalPathPattern = /^\/(?:bookings|specialists|notifications)\/[A-Za-z0-9._~-]+$/;

type FetchOptions = {
  method?: string;
  headers?: Record<string, string>;
  credentials?: "include";
};

type FetchResponse = {
  status: number;
  json(): Promise<unknown>;
};

type FetchLike = (url: string, options?: FetchOptions) => Promise<FetchResponse>;

const defaultFetch: FetchLike = (url, options) => fetch(url, options);

export type NativeDeepLinkAction =
  | { action: "OPEN_APP_PATH"; target: string }
  | { action: "OPEN_WEB_FALLBACK"; target: string }
  | { action: "BLOCK" };

export function extractCanonicalDeepLinkToken(rawURL: string): string | null {
  let parsed: URL;
  try {
    parsed = new URL(rawURL);
  } catch {
    return null;
  }
  if (
    parsed.protocol !== "https:" ||
    parsed.hostname.toLowerCase() !== "apgic.ru" ||
    (parsed.port !== "" && parsed.port !== "443") ||
    parsed.username !== "" ||
    parsed.password !== "" ||
    parsed.search !== "" ||
    parsed.hash !== ""
  ) {
    return null;
  }
  const match = /^\/l\/(v1\.[A-Za-z0-9_-]+)$/.exec(parsed.pathname);
  const token = match?.[1] ?? null;
  return token && token.length <= maxDeepLinkTokenLength ? token : null;
}

function parseResolution(value: unknown): DeepLinkResolution | null {
  if (!value || typeof value !== "object") {
    return null;
  }
  const candidate = value as Partial<DeepLinkResolution>;
  if (
    (candidate.decision !== "ALLOW" && candidate.decision !== "DENY") ||
    typeof candidate.reason_code !== "string" ||
    candidate.reason_code.length === 0
  ) {
    return null;
  }
  if (candidate.decision === "DENY") {
    if (
      candidate.canonical_path !== undefined ||
      candidate.canonical_web_fallback !== undefined ||
      candidate.expires_at !== undefined
    ) {
      return null;
    }
    return candidate as DeepLinkResolution;
  }
  if (
    candidate.reason_code !== "DEEPLINK_ALLOWED" ||
    typeof candidate.canonical_path !== "string" ||
    !canonicalPathPattern.test(candidate.canonical_path) ||
    typeof candidate.canonical_web_fallback !== "string" ||
    typeof candidate.expires_at !== "string" ||
    Number.isNaN(Date.parse(candidate.expires_at))
  ) {
    return null;
  }
  try {
    const fallback = new URL(candidate.canonical_web_fallback);
    if (
      fallback.protocol !== "https:" ||
      fallback.hostname.toLowerCase() !== "apgic.ru" ||
      (fallback.port !== "" && fallback.port !== "443") ||
      fallback.username !== "" ||
      fallback.password !== "" ||
      fallback.search !== "" ||
      fallback.hash !== "" ||
      fallback.pathname !== candidate.canonical_path
    ) {
      return null;
    }
  } catch {
    return null;
  }
  return candidate as DeepLinkResolution;
}

export async function resolveCanonicalUniversalLink(
  rawURL: string,
  options?: {
    apiOrigin?: string;
    sessionCookie?: string;
    selectedWorkspaceID?: string;
    request?: FetchLike;
  },
): Promise<NativeDeepLinkAction> {
  const token = extractCanonicalDeepLinkToken(rawURL);
  if (!token) {
    return {action: "BLOCK"};
  }
  const apiOrigin = (options?.apiOrigin ?? canonicalAPGICOrigin).replace(/\/$/, "");
  if (
    apiOrigin !== canonicalAPGICOrigin &&
    !/^http:\/\/127\.0\.0\.1(?::\d+)?$/.test(apiOrigin)
  ) {
    return {action: "BLOCK"};
  }
  const request = options?.request ?? defaultFetch;
  const headers: Record<string, string> = {Accept: "application/json"};
  if (options?.sessionCookie) {
    headers.Cookie = options.sessionCookie;
  }
  const selectedWorkspaceID = options?.selectedWorkspaceID?.trim();
  if (
    selectedWorkspaceID &&
    (selectedWorkspaceID.length > maxWorkspaceIDLength ||
      !canonicalWorkspaceIDPattern.test(selectedWorkspaceID))
  ) {
    return {action: "BLOCK"};
  }
  const workspaceQuery = selectedWorkspaceID
    ? `&workspace_id=${encodeURIComponent(selectedWorkspaceID)}`
    : "";
  const response = await request(
    `${apiOrigin}/v1/mobile/deep-links/resolve?token=${encodeURIComponent(token)}${workspaceQuery}`,
    {
      method: "GET",
      headers,
      credentials: "include",
    },
  );
  if (response.status !== 200) {
    return {action: "BLOCK"};
  }
  const resolution = parseResolution(await response.json());
  if (!resolution) {
    return {action: "BLOCK"};
  }
  return resolveNativeDeepLink(resolution);
}
