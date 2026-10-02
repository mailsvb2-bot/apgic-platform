import type {
  ClientCompatibilityDecision,
  MobilePlatform,
} from "../../../packages/contracts/src/mobile-policy.ts";

export type CompatibilityRequest = {
  baseURL: string;
  platform: MobilePlatform;
  appVersion: string;
  contractVersion: string;
};

const statuses = new Set([
  "SUPPORTED",
  "DEPRECATED_BUT_SUPPORTED",
  "UPDATE_REQUIRED",
]);
const reasons = new Set([
  "CLIENT_VERSION_SUPPORTED",
  "CLIENT_VERSION_DEPRECATED",
  "CLIENT_VERSION_BELOW_MINIMUM",
  "CLIENT_CONTRACT_UNSUPPORTED",
]);
const updateReasons = new Set([
  "SECURITY_CRITICAL",
  "LEGAL_CRITICAL",
  "INCOMPATIBLE_CRITICAL",
]);

export async function fetchClientCompatibility(
  input: CompatibilityRequest,
): Promise<ClientCompatibilityDecision> {
  const baseURL = input.baseURL.replace(/\/+$/, "");
  const query = [
    ["platform", input.platform],
    ["app_version", input.appVersion],
    ["contract_version", input.contractVersion],
  ]
    .map(([key, value]) => `${key}=${encodeURIComponent(value)}`)
    .join("&");

  const response = await fetch(`${baseURL}/v1/mobile/compatibility?${query}`, {
    method: "GET",
    headers: {accept: "application/json"},
  });
  if (!response.ok) {
    throw new Error(`CLIENT_COMPATIBILITY_HTTP_${response.status}`);
  }

  const payload: unknown = await response.json();
  if (!isCompatibilityDecision(payload)) {
    throw new Error("CLIENT_COMPATIBILITY_RESPONSE_INVALID");
  }
  return payload;
}

export function isCompatibilityDecision(
  value: unknown,
): value is ClientCompatibilityDecision {
  if (!value || typeof value !== "object") {
    return false;
  }
  const decision = value as Record<string, unknown>;
  if (
    typeof decision.status !== "string" ||
    !statuses.has(decision.status) ||
    typeof decision.reason_code !== "string" ||
    !reasons.has(decision.reason_code) ||
    typeof decision.policy_version !== "string" ||
    decision.policy_version.length === 0 ||
    typeof decision.contract_version !== "string" ||
    decision.contract_version.length === 0
  ) {
    return false;
  }

  if (decision.status === "UPDATE_REQUIRED") {
    return (
      typeof decision.update_reason === "string" &&
      updateReasons.has(decision.update_reason) &&
      typeof decision.update_url === "string" &&
      decision.update_url.startsWith("https://")
    );
  }

  return decision.update_reason === undefined && decision.update_url === undefined;
}
