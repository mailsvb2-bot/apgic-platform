import type { DeepLinkResolution } from "../../../packages/contracts/src/generated/apgic-v1";
import { resolveNativeDeepLink } from "./r1-cross-surface.ts";

export const canonicalAPGICOrigin = "https://apgic.ru";

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
  const match = /^\/l\/(v1\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+)$/.exec(parsed.pathname);
  return match?.[1] ?? null;
}

function parseResolution(value: unknown): DeepLinkResolution | null {
  if (!value || typeof value !== "object") {
    return null;
  }
  const candidate = value as Partial<DeepLinkResolution>;
  if (
    (candidate.decision !== "ALLOW" && candidate.decision !== "DENY") ||
    typeof candidate.reason_code !== "string"
  ) {
    return null;
  }
  if (candidate.canonical_path !== undefined && typeof candidate.canonical_path !== "string") {
    return null;
  }
  if (
    candidate.canonical_web_fallback !== undefined &&
    typeof candidate.canonical_web_fallback !== "string"
  ) {
    return null;
  }
  if (candidate.expires_at !== undefined && typeof candidate.expires_at !== "string") {
    return null;
  }
  return candidate as DeepLinkResolution;
}

export async function resolveCanonicalUniversalLink(
  rawURL: string,
  options?: {
    apiOrigin?: string;
    sessionCookie?: string;
    request?: FetchLike;
  },
): Promise<NativeDeepLinkAction> {
  const token = extractCanonicalDeepLinkToken(rawURL);
  if (!token) {
    return {action: "BLOCK"};
  }
  const apiOrigin = (options?.apiOrigin ?? canonicalAPGICOrigin).replace(/\/$/, "");
  if (!/^https:\/\//.test(apiOrigin) && !/^http:\/\/127\.0\.0\.1(?::\d+)?$/.test(apiOrigin)) {
    return {action: "BLOCK"};
  }
  const request = options?.request ?? defaultFetch;
  const headers: Record<string, string> = {Accept: "application/json"};
  if (options?.sessionCookie) {
    headers.Cookie = options.sessionCookie;
  }
  const response = await request(
    `${apiOrigin}/v1/mobile/deep-links/resolve?token=${encodeURIComponent(token)}`,
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
