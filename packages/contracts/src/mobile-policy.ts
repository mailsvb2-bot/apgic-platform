export type ClientPlatform = "IOS" | "ANDROID";

export type ClientCompatibilityStatus =
  | "SUPPORTED"
  | "DEPRECATED_BUT_SUPPORTED"
  | "UPDATE_REQUIRED";

export interface ClientCompatibilityDecision {
  status: ClientCompatibilityStatus;
  reason_code:
    | "CLIENT_VERSION_SUPPORTED"
    | "CLIENT_VERSION_DEPRECATED"
    | "CLIENT_VERSION_BELOW_MINIMUM";
  policy_version: string;
  contract_version: string;
}

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
