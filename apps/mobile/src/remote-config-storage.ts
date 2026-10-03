import {NativeModules} from "react-native";

import type {RemoteConfigStorage} from "./remote-config-client.ts";

type NativeRemoteConfigStorage = {
  loadRemoteConfig(): Promise<string | null>;
  saveRemoteConfig(value: string): Promise<void>;
  clearRemoteConfig(): Promise<void>;
};

function nativeStorage(): NativeRemoteConfigStorage {
  const candidate = (
    NativeModules as Record<string, NativeRemoteConfigStorage | undefined>
  ).APGICOfflineMutationStorage;
  if (!candidate) {
    throw new Error("REMOTE_CONFIG_STORAGE_UNAVAILABLE");
  }
  return candidate;
}

export const remoteConfigStorage: RemoteConfigStorage = {
  load: () => nativeStorage().loadRemoteConfig(),
  save: (value) => nativeStorage().saveRemoteConfig(value),
  clear: () => nativeStorage().clearRemoteConfig(),
};
