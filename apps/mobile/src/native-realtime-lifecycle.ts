import {NativeEventEmitter, NativeModules} from "react-native";

import type {AudioRoute} from "../../../packages/contracts/src/r3-mobile-realtime";
import type {NativeRealtimeEvent} from "./r3-realtime.ts";
import type {NativeRealtimeLifecycleSource} from "./r3-realtime-controller.ts";

type NativeRealtimeLifecycleModule = {
  start(): Promise<void>;
  stop(): Promise<void>;
  debugEmit(type: string, route?: string): Promise<void>;
  addListener(eventName: string): void;
  removeListeners(count: number): void;
};

type NativeLifecyclePayload = {type?: unknown; route?: unknown};

const eventName = "APGICRealtimeLifecycleEvent";
const audioRoutes = new Set<AudioRoute>(["SPEAKER", "EARPIECE", "BLUETOOTH", "WIRED", "UNKNOWN"]);
const simpleTypes = new Set<NativeRealtimeEvent["type"]>([
  "PROVIDER_DISCONNECTED",
  "NETWORK_OFFLINE",
  "NETWORK_DEGRADED",
  "NETWORK_ONLINE",
  "APP_BACKGROUND",
  "APP_FOREGROUND",
  "INTERRUPTION_BEGAN",
  "INTERRUPTION_ENDED",
  "MICROPHONE_PERMISSION_REVOKED",
]);

function module(): NativeRealtimeLifecycleModule {
  const candidate = (NativeModules as Record<string, NativeRealtimeLifecycleModule | undefined>)
    .APGICRealtimeLifecycle;
  if (!candidate) {
    throw new Error("REALTIME_LIFECYCLE_NATIVE_MODULE_UNAVAILABLE");
  }
  return candidate;
}

export function parseNativeRealtimeEvent(payload: NativeLifecyclePayload): NativeRealtimeEvent {
  if (payload.type === "AUDIO_ROUTE_CHANGED") {
    if (typeof payload.route !== "string" || !audioRoutes.has(payload.route as AudioRoute)) {
      throw new Error("REALTIME_NATIVE_AUDIO_ROUTE_INVALID");
    }
    return {type: "AUDIO_ROUTE_CHANGED", route: payload.route as AudioRoute};
  }
  if (typeof payload.type !== "string" || !simpleTypes.has(payload.type as NativeRealtimeEvent["type"])) {
    throw new Error("REALTIME_NATIVE_EVENT_INVALID");
  }
  return {type: payload.type as Exclude<NativeRealtimeEvent["type"], "AUDIO_ROUTE_CHANGED" | "SESSION_OPENED" | "PROVIDER_CONNECTED">} as NativeRealtimeEvent;
}

export class NativeRealtimeLifecycle implements NativeRealtimeLifecycleSource {
  private subscription?: {remove(): void};

  async start(listener: (event: NativeRealtimeEvent) => void): Promise<void> {
    if (this.subscription) {
      return;
    }
    const native = module();
    const emitter = new NativeEventEmitter(native as never);
    this.subscription = emitter.addListener(eventName, (payload: NativeLifecyclePayload) => {
      listener(parseNativeRealtimeEvent(payload));
    });
    await native.start();
  }

  async stop(): Promise<void> {
    const native = module();
    this.subscription?.remove();
    this.subscription = undefined;
    await native.stop();
  }
}

export async function debugEmitNativeRealtimeEvent(event: NativeRealtimeEvent): Promise<void> {
  if (event.type === "SESSION_OPENED" || event.type === "PROVIDER_CONNECTED") {
    throw new Error("REALTIME_DEBUG_EVENT_FORBIDDEN");
  }
  await module().debugEmit(event.type, event.type === "AUDIO_ROUTE_CHANGED" ? event.route : undefined);
}