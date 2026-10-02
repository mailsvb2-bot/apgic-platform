import type {NativeRealtimeSnapshotV1} from "../../../packages/contracts/src/r3-mobile-realtime";
import {
  NativeRealtimeController,
  type CommunicationProviderPort,
} from "./r3-realtime-controller.ts";
import type {NativeRealtimeEvent, NativeRealtimePolicy} from "./r3-realtime.ts";
import {
  NativeRealtimeLifecycle,
  debugEmitNativeRealtimeEvent,
} from "./native-realtime-lifecycle.ts";

export interface NativeRealtimeE2EResult {
  phase: NativeRealtimeSnapshotV1["phase"];
  businessTransitions: string[];
  providerActions: string[];
  audioRoute: NativeRealtimeSnapshotV1["audio_route"];
  appState: NativeRealtimeSnapshotV1["app_state"];
  networkState: NativeRealtimeSnapshotV1["network_state"];
  networkTransport: NativeRealtimeSnapshotV1["network_transport"];
  screenState: NativeRealtimeSnapshotV1["screen_state"];
  joinAuthState: NativeRealtimeSnapshotV1["join_auth_state"];
  consultationId: string;
}

const e2ePolicy: NativeRealtimePolicy = {
  policyVersion: "mobile-realtime-r3-ci-v1",
  maxReconnectAttempts: 2,
  allowBackgroundReconnect: false,
  allowScreenLockedReconnect: false,
};

export function parseRealtimeE2EEvents(value: string): NativeRealtimeEvent[] {
  return value.split(",").filter(Boolean).map((token) => {
    if (token.startsWith("NETWORK_TRANSPORT_CHANGED:")) {
      const transport = token.slice("NETWORK_TRANSPORT_CHANGED:".length);
      if (!["WIFI", "CELLULAR", "ETHERNET", "OTHER", "UNKNOWN"].includes(transport)) {
        throw new Error("REALTIME_E2E_NETWORK_TRANSPORT_INVALID");
      }
      return {type: "NETWORK_TRANSPORT_CHANGED", transport} as NativeRealtimeEvent;
    }
    if (token.startsWith("AUDIO_ROUTE_CHANGED:")) {
      const route = token.slice("AUDIO_ROUTE_CHANGED:".length);
      if (!["SPEAKER", "EARPIECE", "BLUETOOTH", "WIRED", "UNKNOWN"].includes(route)) {
        throw new Error("REALTIME_E2E_AUDIO_ROUTE_INVALID");
      }
      return {type: "AUDIO_ROUTE_CHANGED", route} as NativeRealtimeEvent;
    }
    if ([
      "PROVIDER_DISCONNECTED",
      "NETWORK_OFFLINE",
      "NETWORK_DEGRADED",
      "NETWORK_ONLINE",
      "APP_BACKGROUND",
      "APP_FOREGROUND",
      "SCREEN_LOCKED",
      "SCREEN_UNLOCKED",
      "INTERRUPTION_BEGAN",
      "INTERRUPTION_ENDED",
      "MICROPHONE_PERMISSION_REVOKED",
      "MICROPHONE_PERMISSION_GRANTED",
      "JOIN_AUTH_EXPIRED",
    ].includes(token)) {
      return {type: token} as NativeRealtimeEvent;
    }
    throw new Error("REALTIME_E2E_EVENT_INVALID");
  });
}

function initialSnapshot(consultationId: string): NativeRealtimeSnapshotV1 {
  return {
    contract_version: "native-realtime-v1",
    consultation_id: consultationId,
    phase: "IDLE",
    app_state: "FOREGROUND",
    network_state: "ONLINE",
    network_transport: "WIFI",
    microphone_permission: "GRANTED",
    audio_route: "SPEAKER",
    interruption: "NONE",
    screen_state: "UNLOCKED",
    join_auth_state: "VALID",
    reconnect_attempt: 0,
  };
}

function provider(reconnectFailures: number, actions: string[]): CommunicationProviderPort {
  let reconnectAttempt = 0;
  return {
    async connect() {
      actions.push("CONNECT_PROVIDER");
      return "native-e2e-provider-connection";
    },
    async reconnect() {
      actions.push("RECONNECT_PROVIDER");
      reconnectAttempt += 1;
      if (reconnectAttempt <= reconnectFailures) {
        throw new Error("NATIVE_E2E_PROVIDER_DISCONNECTED");
      }
      return `native-e2e-provider-reconnect-${reconnectAttempt}`;
    },
    async pauseMedia() { actions.push("PAUSE_MEDIA"); },
    async refreshAudioRoute() { actions.push("REFRESH_AUDIO_ROUTE"); },
    async refreshJoinAuth() {
      actions.push("REFRESH_JOIN_AUTH");
      return "native-e2e-provider-auth-refreshed";
    },
    async requestMicrophonePermission() { actions.push("REQUEST_PERMISSION"); },
    async reportTechnicalFailure(reasonCode) { actions.push(`REPORT_TECHNICAL_FAILURE:${reasonCode}`); },
  };
}

async function waitForReduction(count: () => number, previous: number): Promise<void> {
  for (let attempt = 0; attempt < 100; attempt += 1) {
    if (count() > previous) return;
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  throw new Error("REALTIME_E2E_NATIVE_EVENT_TIMEOUT");
}

export async function runNativeRealtimeE2E(
  eventScript: string,
  reconnectFailures = 0,
  consultationId = "consultation-native-e2e",
): Promise<NativeRealtimeE2EResult> {
  const events = parseRealtimeE2EEvents(eventScript);
  const lifecycle = new NativeRealtimeLifecycle();
  const actions: string[] = [];
  const transitions: string[] = [];
  let reductions = 0;
  const controller = new NativeRealtimeController(
    initialSnapshot(consultationId),
    e2ePolicy,
    provider(reconnectFailures, actions),
    lifecycle,
    (result) => {
      reductions += 1;
      transitions.push(result.business_transition);
    },
  );

  await controller.start();
  try {
    for (const event of events) {
      const before = reductions;
      await debugEmitNativeRealtimeEvent(event);
      await waitForReduction(() => reductions, before);
    }
    const snapshot = controller.snapshot();
    if (transitions.some((value) => value !== "NONE")) {
      throw new Error("REALTIME_E2E_FALSE_BUSINESS_TRANSITION");
    }
    return {
      phase: snapshot.phase,
      businessTransitions: transitions,
      providerActions: actions,
      audioRoute: snapshot.audio_route,
      appState: snapshot.app_state,
      networkState: snapshot.network_state,
      networkTransport: snapshot.network_transport,
      screenState: snapshot.screen_state,
      joinAuthState: snapshot.join_auth_state,
      consultationId: snapshot.consultation_id,
    };
  } finally {
    await controller.stop();
  }
}