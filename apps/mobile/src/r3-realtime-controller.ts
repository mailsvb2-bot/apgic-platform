import type {
  NativeRealtimeReductionV1,
  NativeRealtimeSnapshotV1,
} from "../../../packages/contracts/src/r3-mobile-realtime";
import {
  reduceNativeRealtime,
  type NativeRealtimeEvent,
  type NativeRealtimePolicy,
} from "./r3-realtime.ts";

export interface CommunicationProviderPort {
  connect(): Promise<string>;
  reconnect(): Promise<string>;
  pauseMedia(): Promise<void>;
  refreshAudioRoute(): Promise<void>;
  refreshJoinAuth(): Promise<string>;
  requestMicrophonePermission(): Promise<void>;
  reportTechnicalFailure(reasonCode: string): Promise<void>;
}

export interface NativeRealtimeLifecycleSource {
  start(listener: (event: NativeRealtimeEvent) => void): Promise<void> | void;
  stop(): Promise<void> | void;
}

export type NativeRealtimeObserver = (reduction: NativeRealtimeReductionV1) => void;

export class NativeRealtimeController {
  private current: NativeRealtimeSnapshotV1;
  private chain: Promise<void> = Promise.resolve();
  private started = false;
  private readonly policy: NativeRealtimePolicy;
  private readonly provider: CommunicationProviderPort;
  private readonly lifecycle: NativeRealtimeLifecycleSource;
  private readonly observe?: NativeRealtimeObserver;

  constructor(
    initial: NativeRealtimeSnapshotV1,
    policy: NativeRealtimePolicy,
    provider: CommunicationProviderPort,
    lifecycle: NativeRealtimeLifecycleSource,
    observe?: NativeRealtimeObserver,
  ) {
    this.current = {...initial};
    this.policy = policy;
    this.provider = provider;
    this.lifecycle = lifecycle;
    this.observe = observe;
  }

  snapshot(): NativeRealtimeSnapshotV1 {
    return {...this.current};
  }

  async start(): Promise<void> {
    if (this.started) {
      return;
    }
    this.started = true;
    await this.lifecycle.start((event) => {
      void this.dispatch(event);
    });
    await this.dispatch({type: "SESSION_OPENED"});
  }

  async stop(): Promise<void> {
    if (!this.started) {
      return;
    }
    this.started = false;
    await this.lifecycle.stop();
    await this.chain;
  }

  dispatch(event: NativeRealtimeEvent): Promise<void> {
    const next = this.chain.then(() => this.apply(event));
    this.chain = next.catch(() => undefined);
    return next;
  }

  private async apply(event: NativeRealtimeEvent): Promise<void> {
    const reduction = reduceNativeRealtime(this.current, event, this.policy);
    if (reduction.business_transition !== "NONE") {
      throw new Error("REALTIME_BUSINESS_TRANSITION_FORBIDDEN");
    }
    this.current = reduction.snapshot;
    this.observe?.(reduction);
    await this.execute(reduction);
  }

  private async execute(reduction: NativeRealtimeReductionV1): Promise<void> {
    switch (reduction.technical_action) {
      case "NONE":
      case "WAIT_FOR_NETWORK":
        return;
      case "CONNECT_PROVIDER": {
        const ref = await this.provider.connect();
        await this.apply({type: "PROVIDER_CONNECTED", providerConnectionRef: ref});
        return;
      }
      case "RECONNECT_PROVIDER": {
        try {
          const ref = await this.provider.reconnect();
          await this.apply({type: "PROVIDER_CONNECTED", providerConnectionRef: ref});
        } catch {
          await this.apply({type: "PROVIDER_DISCONNECTED"});
        }
        return;
      }
      case "PAUSE_MEDIA":
        await this.provider.pauseMedia();
        return;
      case "REFRESH_AUDIO_ROUTE":
        await this.provider.refreshAudioRoute();
        return;
      case "REFRESH_JOIN_AUTH":
        try {
          const ref = await this.provider.refreshJoinAuth();
          await this.apply({type: "PROVIDER_CONNECTED", providerConnectionRef: ref});
        } catch {
          await this.apply({type: "JOIN_AUTH_REFRESH_FAILED"});
        }
        return;
      case "REQUEST_PERMISSION":
        await this.provider.requestMicrophonePermission();
        return;
      case "REPORT_TECHNICAL_FAILURE":
        await this.provider.reportTechnicalFailure(reduction.reason_code);
        return;
    }
  }
}
