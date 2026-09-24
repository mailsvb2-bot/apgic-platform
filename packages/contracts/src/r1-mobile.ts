export type R1ClientSurface = "WEB" | "IOS" | "ANDROID";

export type DeepLinkDecision = "ALLOW" | "DENY";

export interface DeepLinkResolutionV1 {
  decision: DeepLinkDecision;
  reason_code: string;
  canonical_path?: string;
  canonical_web_fallback?: string;
  expires_at: string;
}

export type AnalyticsPrimitive = string | number | boolean | null;

export type CanonicalAnalyticsEventName =
  | "specialist_discovery_viewed"
  | "workspace_switched"
  | "account_deletion_requested";

export interface CanonicalAnalyticsEventV1 {
  event_name: CanonicalAnalyticsEventName;
  event_version: "1";
  occurred_at: string;
  journey_id: string;
  identity_ref?: string;
  business_properties: Record<string, AnalyticsPrimitive>;
  platform_extensions?: Partial<
    Record<R1ClientSurface, Record<string, AnalyticsPrimitive>>
  >;
}

export type DeleteAccountSource = R1ClientSurface;

export type DeleteAccountState =
  | "REQUESTED"
  | "IDENTITY_RECONFIRMED"
  | "RETENTION_CLASSIFIED"
  | "PROVIDER_ERASURE_PENDING"
  | "WAITING_FOR_LEGAL_HOLD_EXPIRY"
  | "PARTIALLY_RETAINED_WITH_REASON"
  | "COMPLETED";

export interface DeleteAccountInitiationV1 {
  contract_version: "delete-account-v1";
  request_id: string;
  identity_id: string;
  source: DeleteAccountSource;
}

export interface DeleteAccountStatusV1 {
  request_id: string;
  state: DeleteAccountState;
  retained_reason?: string;
}

export type WorkspaceKind = "CLIENT" | "SPECIALIST" | "ORGANIZATION";

export interface AuthorizedWorkspaceV1 {
  workspace_id: string;
  identity_id: string;
  tenant_id: string;
  kind: WorkspaceKind;
  authorization_decision: "ALLOW";
  reason_code: string;
}

export interface WorkspaceResolutionV1 {
  allowed: boolean;
  reason_code: string;
  workspace?: AuthorizedWorkspaceV1;
}

export interface MobileSupportPolicyV1 {
  policy_version: string;
  minimum_ios_major: number;
  minimum_android_api: number;
  accessibility: {
    voice_over: boolean;
    talk_back: boolean;
    text_scaling: boolean;
    reduced_motion: boolean;
    focus_order: boolean;
    touch_target_min_dp: number;
  };
}
