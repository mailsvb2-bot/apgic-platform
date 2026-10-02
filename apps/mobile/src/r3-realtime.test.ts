import assert from "node:assert/strict";
import test from "node:test";

import type { NativeRealtimeSnapshotV1 } from "../../../packages/contracts/src/r3-mobile-realtime";
import { reduceNativeRealtime, type NativeRealtimePolicy } from "./r3-realtime.ts";

const policy: NativeRealtimePolicy = {
  policyVersion: "mobile-realtime-r3-ci-v1",
  maxReconnectAttempts: 2,
  allowBackgroundReconnect: true,
};

function connected(): NativeRealtimeSnapshotV1 {
  return {
    contract_version: "native-realtime-v1",
    consultation_id: "consultation-1",
    phase: "CONNECTED",
    app_state: "FOREGROUND",
    network_state: "ONLINE",
    microphone_permission: "GRANTED",
    audio_route: "SPEAKER",
    interruption: "NONE",
    reconnect_attempt: 0,
    provider_connection_ref: "provider-connection-1",
  };
}

test("network loss enters reconnecting without business completion", () => {
  const offline = reduceNativeRealtime(connected(), { type: "NETWORK_OFFLINE" }, policy);
  assert.equal(offline.snapshot.phase, "RECONNECTING");
  assert.equal(offline.technical_action, "WAIT_FOR_NETWORK");
  assert.equal(offline.business_transition, "NONE");

  const online = reduceNativeRealtime(offline.snapshot, { type: "NETWORK_ONLINE" }, policy);
  assert.equal(online.snapshot.phase, "RECONNECTING");
  assert.equal(online.technical_action, "RECONNECT_PROVIDER");
  assert.equal(online.business_transition, "NONE");
});

test("background, interruption and audio route changes remain technical only", () => {
  let result = reduceNativeRealtime(connected(), { type: "APP_BACKGROUND" }, policy);
  assert.equal(result.business_transition, "NONE");

  result = reduceNativeRealtime(result.snapshot, { type: "AUDIO_ROUTE_CHANGED", route: "BLUETOOTH" }, policy);
  assert.equal(result.snapshot.audio_route, "BLUETOOTH");
  assert.equal(result.business_transition, "NONE");

  result = reduceNativeRealtime(result.snapshot, { type: "INTERRUPTION_BEGAN" }, policy);
  assert.equal(result.snapshot.phase, "DEGRADED");
  assert.equal(result.business_transition, "NONE");
});

test("reconnect exhaustion becomes technical failure, never completed/no-show", () => {
  let snapshot = connected();
  snapshot = { ...snapshot, reconnect_attempt: 2 };
  const result = reduceNativeRealtime(snapshot, { type: "PROVIDER_DISCONNECTED" }, policy);
  assert.equal(result.snapshot.phase, "TECHNICAL_FAILURE");
  assert.equal(result.technical_action, "REPORT_TECHNICAL_FAILURE");
  assert.equal(result.business_transition, "NONE");
});

test("revoked microphone permission blocks media without changing business truth", () => {
  const result = reduceNativeRealtime(connected(), { type: "MICROPHONE_PERMISSION_REVOKED" }, policy);
  assert.equal(result.snapshot.phase, "BLOCKED");
  assert.equal(result.technical_action, "REQUEST_PERMISSION");
  assert.equal(result.business_transition, "NONE");
});


test("permission revocation cannot be bypassed by network recovery", () => {
  const revoked = reduceNativeRealtime(connected(), { type: "MICROPHONE_PERMISSION_REVOKED" }, policy);
  assert.equal(revoked.snapshot.phase, "BLOCKED");
  assert.equal(revoked.technical_action, "REQUEST_PERMISSION");
  const offline = reduceNativeRealtime(revoked.snapshot, { type: "NETWORK_OFFLINE" }, policy);
  const online = reduceNativeRealtime(offline.snapshot, { type: "NETWORK_ONLINE" }, policy);
  assert.equal(online.snapshot.phase, "BLOCKED");
  assert.equal(online.technical_action, "REQUEST_PERMISSION");
  assert.equal(online.snapshot.microphone_permission, "DENIED");
  const restored = reduceNativeRealtime(online.snapshot, { type: "MICROPHONE_PERMISSION_GRANTED" }, policy);
  assert.equal(restored.snapshot.microphone_permission, "GRANTED");
  assert.equal(restored.technical_action, "RECONNECT_PROVIDER");
  assert.equal(restored.business_transition, "NONE");
});

test("initial permission grant starts a fresh provider connection", () => {
  const initial: NativeRealtimeSnapshotV1 = {...connected(), phase: "IDLE", microphone_permission: "DENIED", provider_connection_ref: undefined};
  const opened = reduceNativeRealtime(initial, { type: "SESSION_OPENED" }, policy);
  assert.equal(opened.snapshot.phase, "BLOCKED");
  assert.equal(opened.technical_action, "REQUEST_PERMISSION");
  const granted = reduceNativeRealtime(opened.snapshot, { type: "MICROPHONE_PERMISSION_GRANTED" }, policy);
  assert.equal(granted.snapshot.phase, "CONNECTING");
  assert.equal(granted.technical_action, "CONNECT_PROVIDER");
});

test("active interruption blocks network-triggered reconnect until interruption ends", () => {
  const interrupted = reduceNativeRealtime(connected(), { type: "INTERRUPTION_BEGAN" }, policy);
  const online = reduceNativeRealtime(interrupted.snapshot, { type: "NETWORK_ONLINE" }, policy);
  assert.equal(online.snapshot.phase, "DEGRADED");
  assert.equal(online.technical_action, "PAUSE_MEDIA");
  const resumed = reduceNativeRealtime(online.snapshot, { type: "INTERRUPTION_ENDED" }, policy);
  assert.equal(resumed.snapshot.interruption, "NONE");
  assert.equal(resumed.technical_action, "RECONNECT_PROVIDER");
});


test("permission block survives unrelated lifecycle transitions until explicit grant", () => {
  let result = reduceNativeRealtime(connected(), { type: "MICROPHONE_PERMISSION_REVOKED" }, policy);
  assert.equal(result.snapshot.phase, "BLOCKED");

  for (const event of [
    { type: "NETWORK_OFFLINE" } as const,
    { type: "NETWORK_DEGRADED" } as const,
    { type: "APP_BACKGROUND" } as const,
    { type: "AUDIO_ROUTE_CHANGED", route: "BLUETOOTH" } as const,
    { type: "INTERRUPTION_BEGAN" } as const,
    { type: "NETWORK_ONLINE" } as const,
    { type: "APP_FOREGROUND" } as const,
    { type: "INTERRUPTION_ENDED" } as const,
  ]) {
    result = reduceNativeRealtime(result.snapshot, event, policy);
    assert.equal(result.snapshot.phase, "BLOCKED");
    assert.equal(result.technical_action, "NONE");
    assert.equal(result.business_transition, "NONE");
  }

  result = reduceNativeRealtime(result.snapshot, { type: "MICROPHONE_PERMISSION_GRANTED" }, policy);
  assert.equal(result.snapshot.microphone_permission, "GRANTED");
  assert.notEqual(result.snapshot.phase, "BLOCKED");
});
