import type {
  AuthorizedWorkspaceV1,
  CanonicalAnalyticsEventV1,
  DeleteAccountInitiationV1,
  DeepLinkResolutionV1,
  R1ClientSurface,
  WorkspaceResolutionV1,
} from "../../../packages/contracts/src/r1-mobile";

export type NativeSurface = Extract<R1ClientSurface, "IOS" | "ANDROID">;

export function buildNativeDeleteAccountInitiation(
  requestId: string,
  identityId: string,
  platform: NativeSurface,
): DeleteAccountInitiationV1 {
  return {
    contract_version: "delete-account-v1",
    request_id: requestId,
    identity_id: identityId,
    source: platform,
  };
}

export function resolveNativeDeepLink(
  resolution: DeepLinkResolutionV1,
):
  | { action: "OPEN_APP_PATH"; target: string }
  | { action: "OPEN_WEB_FALLBACK"; target: string }
  | { action: "BLOCK" } {
  if (resolution.decision === "ALLOW" && resolution.canonical_path) {
    return { action: "OPEN_APP_PATH", target: resolution.canonical_path };
  }
  if (resolution.canonical_web_fallback) {
    return {
      action: "OPEN_WEB_FALLBACK",
      target: resolution.canonical_web_fallback,
    };
  }
  return { action: "BLOCK" };
}

export function consumeAuthorizedWorkspace(
  resolution: WorkspaceResolutionV1,
): AuthorizedWorkspaceV1 | null {
  if (
    !resolution.allowed ||
    !resolution.workspace ||
    resolution.workspace.authorization_decision !== "ALLOW"
  ) {
    return null;
  }
  return resolution.workspace;
}

export function withNativeAnalyticsDiagnostics(
  event: Omit<CanonicalAnalyticsEventV1, "platform_extensions">,
  platform: NativeSurface,
  diagnostics: Record<string, string | number | boolean | null>,
): CanonicalAnalyticsEventV1 {
  return {
    ...event,
    platform_extensions: {
      [platform]: diagnostics,
    },
  };
}
