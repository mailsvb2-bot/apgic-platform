import {NativeModules} from "react-native";

import type {OfflineMutationStorage} from "./mobile-offline-checkout.ts";

type NativeOfflineMutationStorage = {
  load(): Promise<string | null>;
  save(value: string): Promise<void>;
  clear(): Promise<void>;
};

function nativeStorage(): NativeOfflineMutationStorage {
  const candidate = (
    NativeModules as Record<string, NativeOfflineMutationStorage | undefined>
  ).APGICOfflineMutationStorage;
  if (!candidate) {
    throw new Error("OFFLINE_MUTATION_STORAGE_UNAVAILABLE");
  }
  return candidate;
}

export const offlineMutationStorage: OfflineMutationStorage = {
  load: () => nativeStorage().load(),
  save: (value) => nativeStorage().save(value),
  clear: () => nativeStorage().clear(),
};
