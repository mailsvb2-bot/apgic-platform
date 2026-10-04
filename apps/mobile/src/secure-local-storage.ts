import {NativeModules} from "react-native";

type NativeSecureLocalStorage = {
  loadCredential(): Promise<string | null>;
  saveCredential(value: string): Promise<void>;
  clearCredential(): Promise<void>;
  clearUserScopedState(): Promise<void>;
};

function nativeStorage(): NativeSecureLocalStorage {
  const candidate = (
    NativeModules as Record<string, NativeSecureLocalStorage | undefined>
  ).APGICOfflineMutationStorage;
  if (!candidate) {
    throw new Error("SECURE_LOCAL_STORAGE_UNAVAILABLE");
  }
  return candidate;
}

export const secureCredentialStorage = {
  load: () => nativeStorage().loadCredential(),
  save: (value: string) => nativeStorage().saveCredential(value),
  clear: () => nativeStorage().clearCredential(),
};

export async function clearUserScopedLocalState(): Promise<void> {
  await nativeStorage().clearUserScopedState();
}
