import type {
  CanonicalAnalyticsEventV1,
  DeleteAccountInitiationV1,
  DeepLinkResolutionV1,
} from "../../../packages/contracts/src/r1-mobile";

export function buildWebDeleteAccountInitiation(
  requestId: string,
  identityId: string,
): DeleteAccountInitiationV1 {
  return {
    contract_version: "delete-account-v1",
    request_id: requestId,
    identity_id: identityId,
    source: "WEB",
  };
}

export function resolveWebDeepLink(
  resolution: DeepLinkResolutionV1,
):
  | { action: "OPEN_CANONICAL_PATH"; target: string }
  | { action: "BLOCK" } {
  if (resolution.decision !== "ALLOW" || !resolution.canonical_path) {
    return { action: "BLOCK" };
  }
  return { action: "OPEN_CANONICAL_PATH", target: resolution.canonical_path };
}

export function withWebAnalyticsDiagnostics(
  event: Omit<CanonicalAnalyticsEventV1, "platform_extensions">,
  diagnostics: Record<string, string | number | boolean | null>,
): CanonicalAnalyticsEventV1 {
  return {
    ...event,
    platform_extensions: {
      WEB: diagnostics,
    },
  };
}
