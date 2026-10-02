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

export interface MobilePolicySnapshot {
  compatibility: ClientCompatibilityDecision;
  remote_config_version: number;
  remote_config_policy_id: string;
  capability_states: RemoteCapabilityState[];
}
