export type RealtimePhase =
  | "IDLE"
  | "CONNECTING"
  | "CONNECTED"
  | "DEGRADED"
  | "RECONNECTING"
  | "BLOCKED"
  | "TECHNICAL_FAILURE";

export type AppLifecycleState = "FOREGROUND" | "BACKGROUND";
export type NetworkLifecycleState = "ONLINE" | "DEGRADED" | "OFFLINE";
export type AudioRoute = "SPEAKER" | "EARPIECE" | "BLUETOOTH" | "WIRED" | "UNKNOWN";
export type InterruptionState = "NONE" | "INTERRUPTED";

export interface NativeRealtimeSnapshotV1 {
  contract_version: "native-realtime-v1";
  consultation_id: string;
  phase: RealtimePhase;
  app_state: AppLifecycleState;
  network_state: NetworkLifecycleState;
  microphone_permission: "GRANTED" | "DENIED" | "RESTRICTED" | "UNAVAILABLE";
  audio_route: AudioRoute;
  interruption: InterruptionState;
  reconnect_attempt: number;
  provider_connection_ref?: string;
}

export type NativeRealtimeTechnicalAction =
  | "NONE"
  | "CONNECT_PROVIDER"
  | "RECONNECT_PROVIDER"
  | "WAIT_FOR_NETWORK"
  | "PAUSE_MEDIA"
  | "REFRESH_AUDIO_ROUTE"
  | "REQUEST_PERMISSION"
  | "REPORT_TECHNICAL_FAILURE";

export interface NativeRealtimeReductionV1 {
  snapshot: NativeRealtimeSnapshotV1;
  technical_action: NativeRealtimeTechnicalAction;
  reason_code: string;
  business_transition: "NONE";
}
