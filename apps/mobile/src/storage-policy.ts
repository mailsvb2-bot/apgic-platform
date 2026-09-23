export type LocalDataClass =
  | "PUBLIC"
  | "INTERNAL"
  | "SENSITIVE"
  | "RAW_CONSULTATION"
  | "RAW_PERSONA"
  | "FINANCIAL_EVIDENCE"
  | "CREDENTIAL";

export type StorageTarget =
  | "MEMORY"
  | "EPHEMERAL_CACHE"
  | "SECURE_STORAGE"
  | "PERSISTENT_CACHE";

export function canPersistLocally(
  dataClass: LocalDataClass,
  target: StorageTarget,
): boolean {
  if (dataClass === "CREDENTIAL") {
    return target === "SECURE_STORAGE";
  }
  if (
    dataClass === "RAW_CONSULTATION" ||
    dataClass === "RAW_PERSONA" ||
    dataClass === "FINANCIAL_EVIDENCE"
  ) {
    return target === "MEMORY" || target === "EPHEMERAL_CACHE";
  }
  return true;
}
