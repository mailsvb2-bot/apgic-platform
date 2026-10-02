import type {
  AudioRoute,
  NetworkTransport,
  NativeRealtimeReductionV1,
  NativeRealtimeSnapshotV1,
} from "../../../packages/contracts/src/r3-mobile-realtime";

export interface NativeRealtimePolicy {
  policyVersion: string;
  maxReconnectAttempts: number;
  allowBackgroundReconnect: boolean;
  allowScreenLockedReconnect: boolean;
}

export type NativeRealtimeEvent =
  | { type: "SESSION_OPENED" }
  | { type: "PROVIDER_CONNECTED"; providerConnectionRef: string }
  | { type: "PROVIDER_DISCONNECTED" }
  | { type: "NETWORK_OFFLINE" }
  | { type: "NETWORK_DEGRADED" }
  | { type: "NETWORK_ONLINE" }
  | { type: "NETWORK_TRANSPORT_CHANGED"; transport: NetworkTransport }
  | { type: "APP_BACKGROUND" }
  | { type: "APP_FOREGROUND" }
  | { type: "SCREEN_LOCKED" }
  | { type: "SCREEN_UNLOCKED" }
  | { type: "AUDIO_ROUTE_CHANGED"; route: AudioRoute }
  | { type: "INTERRUPTION_BEGAN" }
  | { type: "INTERRUPTION_ENDED" }
  | { type: "MICROPHONE_PERMISSION_REVOKED" }
  | { type: "MICROPHONE_PERMISSION_GRANTED" }
  | { type: "JOIN_AUTH_EXPIRED" }
  | { type: "JOIN_AUTH_REFRESH_FAILED" };

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

function permissionBlocked(
  snapshot: NativeRealtimeSnapshotV1,
  reasonCode = "REALTIME_MICROPHONE_REQUIRED",
): NativeRealtimeReductionV1 {
  return reduce(snapshot, { phase: "BLOCKED" }, "REQUEST_PERMISSION", reasonCode);
}

function resumeConnection(
  snapshot: NativeRealtimeSnapshotV1,
  policy: NativeRealtimePolicy,
  reasonCode: string,
): NativeRealtimeReductionV1 {
  if (snapshot.microphone_permission !== "GRANTED") {
    return permissionBlocked(snapshot);
  }
  if (snapshot.interruption === "INTERRUPTED") {
    return reduce(snapshot, { phase: "DEGRADED" }, "PAUSE_MEDIA", "REALTIME_INTERRUPTED");
  }
  if (snapshot.network_state === "OFFLINE") {
    return reduce(snapshot, { phase: "RECONNECTING" }, "WAIT_FOR_NETWORK", reasonCode);
  }
  if (snapshot.app_state === "BACKGROUND" && !policy.allowBackgroundReconnect) {
    return reduce(snapshot, { phase: "DEGRADED" }, "PAUSE_MEDIA", "REALTIME_BACKGROUND_PAUSED");
  }
  if (snapshot.screen_state === "LOCKED" && !policy.allowScreenLockedReconnect) {
    return reduce(snapshot, { phase: "DEGRADED" }, "PAUSE_MEDIA", "REALTIME_SCREEN_LOCKED");
  }
  if (snapshot.join_auth_state === "EXPIRED") {
    return reduce(snapshot, { phase: "RECONNECTING" }, "REFRESH_JOIN_AUTH", "REALTIME_JOIN_AUTH_EXPIRED");
  }
  if (!snapshot.provider_connection_ref) {
    return reduce(snapshot, { phase: "CONNECTING", reconnect_attempt: 0 }, "CONNECT_PROVIDER", reasonCode);
  }
  if (snapshot.reconnect_attempt >= policy.maxReconnectAttempts) {
    return reduce(snapshot, { phase: "TECHNICAL_FAILURE" }, "REPORT_TECHNICAL_FAILURE", "REALTIME_RECONNECT_EXHAUSTED");
  }
  return reduce(snapshot, { phase: "RECONNECTING", reconnect_attempt: snapshot.reconnect_attempt + 1 }, "RECONNECT_PROVIDER", reasonCode);
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
      return resumeConnection(snapshot, policy, "REALTIME_CONNECT");
    case "PROVIDER_CONNECTED":
      return reduce(
        snapshot,
        {
          phase: "CONNECTED",
          reconnect_attempt: 0,
          provider_connection_ref: event.providerConnectionRef,
          join_auth_state: "VALID",
        },
        "NONE",
        "REALTIME_CONNECTED",
      );
    case "PROVIDER_DISCONNECTED":
      return resumeConnection(snapshot, policy, "REALTIME_PROVIDER_DISCONNECTED");
    case "NETWORK_OFFLINE":
      if (snapshot.phase === "IDLE" || snapshot.phase === "BLOCKED") {
        return reduce(snapshot, { network_state: "OFFLINE" }, "NONE", "REALTIME_NETWORK_OFFLINE");
      }
      return reduce(
        snapshot,
        { network_state: "OFFLINE", phase: "RECONNECTING" },
        "WAIT_FOR_NETWORK",
        "REALTIME_NETWORK_OFFLINE",
      );
    case "NETWORK_DEGRADED":
      return reduce(
        snapshot,
        snapshot.phase === "IDLE" || snapshot.phase === "BLOCKED"
          ? { network_state: "DEGRADED" }
          : { network_state: "DEGRADED", phase: "DEGRADED" },
        "NONE",
        "REALTIME_NETWORK_DEGRADED",
      );
    case "NETWORK_ONLINE": {
      const online = { ...snapshot, network_state: "ONLINE" as const };
      if (snapshot.phase === "RECONNECTING" || snapshot.phase === "DEGRADED") {
        return resumeConnection(online, policy, "REALTIME_NETWORK_RESTORED");
      }
      return reduce(online, {}, "NONE", "REALTIME_NETWORK_ONLINE");
    }
    case "NETWORK_TRANSPORT_CHANGED": {
      const changed = { ...snapshot, network_transport: event.transport };
      if (
        event.transport === snapshot.network_transport ||
        snapshot.phase === "IDLE" ||
        snapshot.phase === "BLOCKED" ||
        snapshot.phase === "CONNECTING" ||
        snapshot.phase === "TECHNICAL_FAILURE"
      ) {
        return reduce(changed, {}, "NONE", "REALTIME_NETWORK_TRANSPORT_CHANGED");
      }
      return resumeConnection(changed, policy, "REALTIME_NETWORK_TRANSPORT_CHANGED");
    }
    case "APP_BACKGROUND":
      if (snapshot.phase === "BLOCKED") {
        return reduce(snapshot, { app_state: "BACKGROUND" }, "NONE", "REALTIME_APP_BACKGROUND");
      }
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
        return resumeConnection(foreground, policy, "REALTIME_APP_FOREGROUND_RECONNECT");
      }
      return reduce(foreground, {}, "NONE", "REALTIME_APP_FOREGROUND");
    }
    case "SCREEN_LOCKED":
      if (
        snapshot.phase === "IDLE" ||
        snapshot.phase === "BLOCKED" ||
        policy.allowScreenLockedReconnect
      ) {
        return reduce(snapshot, { screen_state: "LOCKED" }, "NONE", "REALTIME_SCREEN_LOCKED");
      }
      return reduce(
        snapshot,
        { screen_state: "LOCKED", phase: "DEGRADED" },
        "PAUSE_MEDIA",
        "REALTIME_SCREEN_LOCKED",
      );
    case "SCREEN_UNLOCKED": {
      const unlocked = { ...snapshot, screen_state: "UNLOCKED" as const };
      if (snapshot.phase === "DEGRADED" || snapshot.phase === "RECONNECTING") {
        return resumeConnection(unlocked, policy, "REALTIME_SCREEN_UNLOCKED");
      }
      return reduce(unlocked, {}, "NONE", "REALTIME_SCREEN_UNLOCKED");
    }
    case "AUDIO_ROUTE_CHANGED":
      return reduce(
        snapshot,
        { audio_route: event.route },
        snapshot.phase === "IDLE" || snapshot.phase === "BLOCKED" || !snapshot.provider_connection_ref
          ? "NONE"
          : "REFRESH_AUDIO_ROUTE",
        "REALTIME_AUDIO_ROUTE_CHANGED",
      );
    case "INTERRUPTION_BEGAN":
      if (snapshot.phase === "IDLE" || snapshot.phase === "BLOCKED") {
        return reduce(snapshot, { interruption: "INTERRUPTED" }, "NONE", "REALTIME_INTERRUPTED");
      }
      return reduce(
        snapshot,
        { interruption: "INTERRUPTED", phase: "DEGRADED" },
        "PAUSE_MEDIA",
        "REALTIME_INTERRUPTED",
      );
    case "INTERRUPTION_ENDED": {
      const resumed = { ...snapshot, interruption: "NONE" as const };
      if (snapshot.phase === "IDLE" || snapshot.phase === "BLOCKED") {
        return reduce(resumed, {}, "NONE", "REALTIME_INTERRUPTION_ENDED");
      }
      if (snapshot.interruption === "INTERRUPTED") {
        return resumeConnection(resumed, policy, "REALTIME_INTERRUPTION_ENDED");
      }
      return reduce(resumed, {}, "NONE", "REALTIME_INTERRUPTION_ENDED");
    }
    case "MICROPHONE_PERMISSION_REVOKED":
      if (snapshot.phase === "IDLE") {
        return reduce(snapshot, { microphone_permission: "DENIED" }, "NONE", "REALTIME_MICROPHONE_REVOKED");
      }
      return reduce(
        snapshot,
        { microphone_permission: "DENIED", phase: "BLOCKED" },
        "REQUEST_PERMISSION",
        "REALTIME_MICROPHONE_REVOKED",
      );
    case "MICROPHONE_PERMISSION_GRANTED": {
      const granted = { ...snapshot, microphone_permission: "GRANTED" as const };
      if (snapshot.phase === "IDLE") {
        return reduce(granted, {}, "NONE", "REALTIME_MICROPHONE_GRANTED");
      }
      if (snapshot.phase === "BLOCKED") {
        return resumeConnection(granted, policy, "REALTIME_MICROPHONE_RESTORED");
      }
      return reduce(granted, {}, "NONE", "REALTIME_MICROPHONE_GRANTED");
    }
    case "JOIN_AUTH_EXPIRED": {
      const expired = { ...snapshot, join_auth_state: "EXPIRED" as const };
      if (snapshot.phase === "IDLE" || snapshot.phase === "BLOCKED") {
        return reduce(expired, {}, "NONE", "REALTIME_JOIN_AUTH_EXPIRED");
      }
      return resumeConnection(expired, policy, "REALTIME_JOIN_AUTH_EXPIRED");
    }
    case "JOIN_AUTH_REFRESH_FAILED":
      return reduce(
        snapshot,
        { phase: "TECHNICAL_FAILURE" },
        "REPORT_TECHNICAL_FAILURE",
        "REALTIME_JOIN_AUTH_REFRESH_FAILED",
      );
  }
}
