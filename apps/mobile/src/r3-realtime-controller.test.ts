import assert from "node:assert/strict";
import test from "node:test";

import type {NativeRealtimeSnapshotV1} from "../../../packages/contracts/src/r3-mobile-realtime";
import {
  NativeRealtimeController,
  type CommunicationProviderPort,
  type NativeRealtimeLifecycleSource,
} from "./r3-realtime-controller.ts";
import type {NativeRealtimeEvent, NativeRealtimePolicy} from "./r3-realtime.ts";

const policy: NativeRealtimePolicy = {
  policyVersion: "mobile-realtime-r3-ci-v1",
  maxReconnectAttempts: 2,
  allowBackgroundReconnect: false,
  allowScreenLockedReconnect: false,
};

function initial(): NativeRealtimeSnapshotV1 {
  return {
    contract_version: "native-realtime-v1",
    consultation_id: "consultation-e2e",
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

class Lifecycle implements NativeRealtimeLifecycleSource {
  listener?: (event: NativeRealtimeEvent) => void;
  start(listener: (event: NativeRealtimeEvent) => void): void { this.listener = listener; }
  stop(): void { this.listener = undefined; }
}

function provider(options: {reconnectFailures?: number} = {}) {
  const actions: string[] = [];
  let reconnects = 0;
  const port: CommunicationProviderPort = {
    async connect() { actions.push("connect"); return "provider-connection-1"; },
    async reconnect() {
      actions.push("reconnect");
      reconnects += 1;
      if (reconnects <= (options.reconnectFailures ?? 0)) {
        throw new Error("provider unavailable");
      }
      return `provider-reconnection-${reconnects}`;
    },
    async pauseMedia() { actions.push("pause"); },
    async refreshAudioRoute() { actions.push("route"); },
    async refreshJoinAuth() { actions.push("auth"); return "provider-auth-refreshed"; },
    async requestMicrophonePermission() { actions.push("permission"); },
    async reportTechnicalFailure(reason) { actions.push(`technical:${reason}`); },
  };
  return {port, actions};
}

test("native lifecycle reconnects provider without business completion", async () => {
  const lifecycle = new Lifecycle();
  const p = provider();
  const business: string[] = [];
  const controller = new NativeRealtimeController(initial(), policy, p.port, lifecycle, (result) => {
    business.push(result.business_transition);
  });

  await controller.start();
  assert.equal(controller.snapshot().phase, "CONNECTED");
  await controller.dispatch({type: "NETWORK_OFFLINE"});
  assert.equal(controller.snapshot().phase, "RECONNECTING");
  await controller.dispatch({type: "NETWORK_ONLINE"});
  assert.equal(controller.snapshot().phase, "CONNECTED");
  assert.deepEqual(p.actions, ["connect", "reconnect"]);
  assert.ok(business.every((value) => value === "NONE"));
});

test("background audio route and interruption stay technical", async () => {
  const lifecycle = new Lifecycle();
  const p = provider();
  const controller = new NativeRealtimeController(initial(), policy, p.port, lifecycle);
  await controller.start();

  await controller.dispatch({type: "APP_BACKGROUND"});
  assert.equal(controller.snapshot().phase, "DEGRADED");
  await controller.dispatch({type: "AUDIO_ROUTE_CHANGED", route: "BLUETOOTH"});
  await controller.dispatch({type: "APP_FOREGROUND"});
  await controller.dispatch({type: "INTERRUPTION_BEGAN"});
  await controller.dispatch({type: "INTERRUPTION_ENDED"});
  assert.ok(p.actions.includes("pause"));
  assert.ok(p.actions.includes("route"));
  assert.ok(p.actions.includes("reconnect"));
});

test("reconnect exhaustion reports technical failure only", async () => {
  const lifecycle = new Lifecycle();
  const p = provider({reconnectFailures: 3});
  const controller = new NativeRealtimeController(initial(), policy, p.port, lifecycle);
  await controller.start();
  await controller.dispatch({type: "PROVIDER_DISCONNECTED"});
  assert.equal(controller.snapshot().phase, "TECHNICAL_FAILURE");
  assert.ok(p.actions.includes("technical:REALTIME_RECONNECT_EXHAUSTED"));
});


test("controller never reconnects while microphone permission is denied", async () => {
  const lifecycle = new Lifecycle();
  const p = provider();
  const denied = {...initial(), microphone_permission: "DENIED" as const};
  const controller = new NativeRealtimeController(denied, policy, p.port, lifecycle);
  await controller.start();
  assert.equal(controller.snapshot().phase, "BLOCKED");
  assert.deepEqual(p.actions, ["permission"]);
  await controller.dispatch({type: "NETWORK_OFFLINE"});
  await controller.dispatch({type: "NETWORK_ONLINE"});
  assert.equal(controller.snapshot().phase, "BLOCKED");
  assert.equal(p.actions.includes("reconnect"), false);
  await controller.dispatch({type: "MICROPHONE_PERMISSION_GRANTED"});
  assert.equal(controller.snapshot().phase, "CONNECTED");
  assert.equal(controller.snapshot().microphone_permission, "GRANTED");
  assert.deepEqual(p.actions, ["permission", "connect"]);
});


test("join auth expiry refreshes auth and rejoins without business completion", async () => {
  const lifecycle = new Lifecycle();
  const p = provider();
  const business: string[] = [];
  const controller = new NativeRealtimeController(initial(), policy, p.port, lifecycle, (result) => {
    business.push(result.business_transition);
  });
  await controller.start();
  await controller.dispatch({type: "JOIN_AUTH_EXPIRED"});
  assert.equal(controller.snapshot().phase, "CONNECTED");
  assert.equal(controller.snapshot().join_auth_state, "VALID");
  assert.equal(controller.snapshot().provider_connection_ref, "provider-auth-refreshed");
  assert.ok(p.actions.includes("auth"));
  assert.ok(business.every((value) => value === "NONE"));
});


test("settle waits for lifecycle-triggered reconnect to reach the stable provider state", async () => {
  const lifecycle = new Lifecycle();
  let releaseReconnect: ((value: string) => void) | undefined;
  let reconnectStarted = false;
  const actions: string[] = [];
  const slowProvider: CommunicationProviderPort = {
    async connect() {
      actions.push("connect");
      return "provider-connection-1";
    },
    async reconnect() {
      actions.push("reconnect");
      reconnectStarted = true;
      return await new Promise<string>((resolve) => {
        releaseReconnect = resolve;
      });
    },
    async pauseMedia() { actions.push("pause"); },
    async refreshAudioRoute() { actions.push("route"); },
    async refreshJoinAuth() { actions.push("auth"); return "provider-auth-refreshed"; },
    async requestMicrophonePermission() { actions.push("permission"); },
    async reportTechnicalFailure(reason) { actions.push(`technical:${reason}`); },
  };
  const controller = new NativeRealtimeController(initial(), policy, slowProvider, lifecycle);
  await controller.start();
  assert.equal(controller.snapshot().phase, "CONNECTED");

  lifecycle.listener?.({type: "NETWORK_OFFLINE"});
  await controller.settle();
  assert.equal(controller.snapshot().phase, "RECONNECTING");

  lifecycle.listener?.({type: "NETWORK_ONLINE"});
  for (let attempt = 0; attempt < 100 && !reconnectStarted; attempt += 1) {
    await new Promise((resolve) => setTimeout(resolve, 1));
  }
  assert.equal(reconnectStarted, true);
  assert.equal(controller.snapshot().phase, "RECONNECTING");

  const settled = controller.settle();
  assert.ok(releaseReconnect);
  releaseReconnect?.("provider-rejoined");
  await settled;

  assert.equal(controller.snapshot().phase, "CONNECTED");
  assert.equal(controller.snapshot().provider_connection_ref, "provider-rejoined");
  assert.deepEqual(actions, ["connect", "reconnect"]);
  await controller.stop();
});
