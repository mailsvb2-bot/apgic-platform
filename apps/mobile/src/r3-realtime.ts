import type {
  AudioRoute,
  NativeRealtimeReductionV1,
  NativeRealtimeSnapshotV1,
} from "../../../packages/contracts/src/r3-mobile-realtime";

export interface NativeRealtimePolicy {
  policyVersion: string;
  maxReconnectAttempts: number;
  allowBackgroundReconnect: boolean;
}

export type NativeRealtimeEvent =
  | { type: "SESSION_OPENED" }
  | { type: "PROVIDER_CONNECTED"; providerConnectionRef: string }
  | { type: "PROVIDER_DISCONNECTED" }
  | { type: "NETWORK_OFFLINE" }
  | { type: "NETWORK_DEGRADED" }
  | { type: "NETWORK_ONLINE" }
  | { type: "APP_BACKGROUND" }
  | { type: "APP_FOREGROUND" }
  | { type: "AUDIO_ROUTE_CHANGED"; route: AudioRoute }
  | { type: "INTERRUPTION_BEGAN" }
  | { type: "INTERRUPTION_ENDED" }
  | { type: "MICROPHONE_PERMISSION_REVOKED" };

function reduce(
  snapshot: NativeRealtimeSnapshotV1,
  patch: Partial<NativeRealtimeSnapshotV1>,
  technicalAction: NativeRealtimeReductionV1["technical_action"],
  reasonCode: string,
): NativeRealtimeReductionV1 {
  return {
    snapshot: { ...snapshot, ...patch },
    technical_action: technicalAction,
    reason_code: reasonCode,
    business_transition: "NONE",
  };
}

function reconnect(
  snapshot: NativeRealtimeSnapshotV1,
  policy: NativeRealtimePolicy,
  reasonCode: string,
): NativeRealtimeReductionV1 {
  if (snapshot.network_state === "OFFLINE") {
    return reduce(snapshot, { phase: "RECONNECTING" }, "WAIT_FOR_NETWORK", reasonCode);
  }
  if (snapshot.app_state === "BACKGROUND" && !policy.allowBackgroundReconnect) {
    return reduce(snapshot, { phase: "DEGRADED" }, "PAUSE_MEDIA", "REALTIME_BACKGROUND_PAUSED");
  }
  if (snapshot.reconnect_attempt >= policy.maxReconnectAttempts) {
    return reduce(
      snapshot,
      { phase: "TECHNICAL_FAILURE" },
      "REPORT_TECHNICAL_FAILURE",
      "REALTIME_RECONNECT_EXHAUSTED",
    );
  }
  return reduce(
    snapshot,
    {
      phase: "RECONNECTING",
      reconnect_attempt: snapshot.reconnect_attempt + 1,
    },
    "RECONNECT_PROVIDER",
    reasonCode,
  );
}

export function reduceNativeRealtime(
  snapshot: NativeRealtimeSnapshotV1,
  event: NativeRealtimeEvent,
  policy: NativeRealtimePolicy,
): NativeRealtimeReductionV1 {
  if (!policy.policyVersion || policy.maxReconnectAttempts < 0) {
    throw new Error("invalid native realtime policy");
  }

  switch (event.type) {
    case "SESSION_OPENED":
      if (snapshot.microphone_permission !== "GRANTED") {
        return reduce(snapshot, { phase: "BLOCKED" }, "REQUEST_PERMISSION", "REALTIME_MICROPHONE_REQUIRED");
      }
      return reduce(snapshot, { phase: "CONNECTING" }, "CONNECT_PROVIDER", "REALTIME_CONNECT");
    case "PROVIDER_CONNECTED":
      return reduce(
        snapshot,
        {
          phase: "CONNECTED",
          reconnect_attempt: 0,
          provider_connection_ref: event.providerConnectionRef,
        },
        "NONE",
        "REALTIME_CONNECTED",
      );
    case "PROVIDER_DISCONNECTED":
      return reconnect(snapshot, policy, "REALTIME_PROVIDER_DISCONNECTED");
    case "NETWORK_OFFLINE":
      return reduce(
        snapshot,
        { network_state: "OFFLINE", phase: "RECONNECTING" },
        "WAIT_FOR_NETWORK",
        "REALTIME_NETWORK_OFFLINE",
      );
    case "NETWORK_DEGRADED":
      return reduce(
        snapshot,
        { network_state: "DEGRADED", phase: "DEGRADED" },
        "NONE",
        "REALTIME_NETWORK_DEGRADED",
      );
    case "NETWORK_ONLINE": {
      const online = { ...snapshot, network_state: "ONLINE" as const };
      if (snapshot.phase === "RECONNECTING" || snapshot.phase === "DEGRADED") {
        return reconnect(online, policy, "REALTIME_NETWORK_RESTORED");
      }
      return reduce(online, {}, "NONE", "REALTIME_NETWORK_ONLINE");
    }
    case "APP_BACKGROUND":
      if (!policy.allowBackgroundReconnect && snapshot.phase !== "IDLE") {
        return reduce(
          snapshot,
          { app_state: "BACKGROUND", phase: "DEGRADED" },
          "PAUSE_MEDIA",
          "REALTIME_BACKGROUND_PAUSED",
        );
      }
      return reduce(snapshot, { app_state: "BACKGROUND" }, "NONE", "REALTIME_APP_BACKGROUND");
    case "APP_FOREGROUND": {
      const foreground = { ...snapshot, app_state: "FOREGROUND" as const };
      if (snapshot.phase === "DEGRADED" || snapshot.phase === "RECONNECTING") {
        return reconnect(foreground, policy, "REALTIME_APP_FOREGROUND_RECONNECT");
      }
      return reduce(foreground, {}, "NONE", "REALTIME_APP_FOREGROUND");
    }
    case "AUDIO_ROUTE_CHANGED":
      return reduce(
        snapshot,
        { audio_route: event.route },
        "REFRESH_AUDIO_ROUTE",
        "REALTIME_AUDIO_ROUTE_CHANGED",
      );
    case "INTERRUPTION_BEGAN":
      return reduce(
        snapshot,
        { interruption: "INTERRUPTED", phase: "DEGRADED" },
        "PAUSE_MEDIA",
        "REALTIME_INTERRUPTED",
      );
    case "INTERRUPTION_ENDED": {
      const resumed = { ...snapshot, interruption: "NONE" as const };
      return reconnect(resumed, policy, "REALTIME_INTERRUPTION_ENDED");
    }
    case "MICROPHONE_PERMISSION_REVOKED":
      return reduce(
        snapshot,
        { microphone_permission: "DENIED", phase: "BLOCKED" },
        "REQUEST_PERMISSION",
        "REALTIME_MICROPHONE_REVOKED",
      );
  }
}
