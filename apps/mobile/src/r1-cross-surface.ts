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
  if (resolution.decision !== "ALLOW") {
    return { action: "BLOCK" };
  }
  if (resolution.canonical_path) {
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


export type NativeDeletionE2EResult = {
  requestID: string;
  identityID: string;
  state: "PARTIALLY_RETAINED_WITH_REASON";
  profileErased: true;
  ledgerRetained: true;
  deactivation: false;
  providerEvidence: string;
  replayIdempotent: true;
};

type AccountDeletionResponse = {
  id: string;
  identity_id: string;
  source: NativeSurface;
  state: string;
  deactivation: boolean;
  profile_erased: boolean;
  ledger_retained: boolean;
  provider_evidence: string;
  apgic_deletes_ledger: boolean;
  idempotent: boolean;
};

async function postNativeDeletion(
  baseURL: string,
  sessionCookie: string,
  identityID: string,
  platform: NativeSurface,
): Promise<AccountDeletionResponse> {
  const response = await fetch(`${baseURL.replace(/\/$/, "")}/v1/account-deletions`, {
    method: "POST",
    headers: {
      "content-type": "application/json",
      cookie: sessionCookie,
      "x-correlation-id": `native-deletion:${platform.toLowerCase()}:${identityID}`,
    },
    body: JSON.stringify({
      identity_id: identityID,
      source: platform,
    }),
  });
  const payload = (await response.json()) as AccountDeletionResponse & {
    message_safe?: string;
  };
  if (!response.ok) {
    throw new Error(payload.message_safe || "NATIVE_ACCOUNT_DELETION_FAILED");
  }
  return payload;
}

export async function runNativeDeletionE2E(options: {
  baseURL: string;
  sessionCookie: string;
  requestID: string;
  identityID: string;
  platform: NativeSurface;
}): Promise<NativeDeletionE2EResult> {
  const initiation = buildNativeDeleteAccountInitiation(
    options.requestID,
    options.identityID,
    options.platform,
  );
  if (
    initiation.contract_version !== "delete-account-v1" ||
    initiation.identity_id !== options.identityID ||
    initiation.source !== options.platform
  ) {
    throw new Error("NATIVE_DELETION_CONTRACT_MISMATCH");
  }

  const first = await postNativeDeletion(
    options.baseURL,
    options.sessionCookie,
    options.identityID,
    options.platform,
  );
  if (
    first.identity_id !== options.identityID ||
    first.source !== options.platform ||
    first.state !== "PARTIALLY_RETAINED_WITH_REASON" ||
    first.deactivation ||
    !first.profile_erased ||
    !first.ledger_retained ||
    first.apgic_deletes_ledger ||
    !first.provider_evidence ||
    first.idempotent
  ) {
    throw new Error("NATIVE_DELETION_CANONICAL_STATE_INVALID");
  }

  const replay = await postNativeDeletion(
    options.baseURL,
    options.sessionCookie,
    options.identityID,
    options.platform,
  );
  if (
    replay.id !== first.id ||
    replay.identity_id !== first.identity_id ||
    replay.state !== first.state ||
    replay.source !== first.source ||
    !replay.idempotent ||
    replay.apgic_deletes_ledger ||
    !replay.ledger_retained
  ) {
    throw new Error("NATIVE_DELETION_REPLAY_NOT_IDEMPOTENT");
  }

  return {
    requestID: initiation.request_id,
    identityID: first.identity_id,
    state: "PARTIALLY_RETAINED_WITH_REASON",
    profileErased: true,
    ledgerRetained: true,
    deactivation: false,
    providerEvidence: first.provider_evidence,
    replayIdempotent: true,
  };
}
