import type {ClientCompatibilityDecision} from "./generated/apgic-v1";

export type MobilePlatform = "IOS" | "ANDROID";

export type ClientCompatibilityStatus =
  | "SUPPORTED"
  | "DEPRECATED_BUT_SUPPORTED"
  | "UPDATE_REQUIRED";

export type ForcedUpdateReason =
  | "SECURITY_CRITICAL"
  | "LEGAL_CRITICAL"
  | "INCOMPATIBLE_CRITICAL";

export type RemoteCapability =
  | "REALTIME_CONSULTATION"
  | "CALENDAR_INTEGRATION"
  | "PERSONA_PREVIEW";

export interface RemoteCapabilityState {
  capability: RemoteCapability;
  disabled: boolean;
  reason_code?: string;
}

export interface RemoteConfigPayload {
  version: number;
  issued_at: string;
  expires_at: string;
  policy_id: string;
  disabled_capabilities: RemoteCapability[];
  reason_codes?: Partial<Record<RemoteCapability, string>>;
}

export interface SignedRemoteConfigEnvelope {
  key_id: string;
  payload: RemoteConfigPayload;
  signature: string;
}

export interface MobilePolicySnapshot {
  compatibility: ClientCompatibilityDecision;
  remote_config_version: number;
  remote_config_policy_id: string;
  capability_states: RemoteCapabilityState[];
}
