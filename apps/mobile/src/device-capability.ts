export type DeviceCapability =
  | "CAMERA"
  | "MICROPHONE"
  | "CALENDAR"
  | "SECURE_STORAGE"
  | "PUSH_NOTIFICATIONS"
  | "UNIVERSAL_LINKS"
  | "SHARE_SHEET"
  | "FILE_PICKER"
  | "PHOTO_PICKER"
  | "BIOMETRIC"
  | "NETWORK_STATUS"
  | "AUDIO_ROUTE"
  | "BACKGROUND_LIFECYCLE";

export type CapabilityState =
  | "UNKNOWN"
  | "NOT_REQUESTED"
  | "GRANTED"
  | "DENIED"
  | "RESTRICTED"
  | "UNAVAILABLE";

export type CapabilityDecision =
  | { state: "GRANTED"; action: "CONTINUE" }
  | { state: "NOT_REQUESTED"; action: "REQUEST_PERMISSION" }
  | { state: "UNKNOWN"; action: "REFRESH_STATE" }
  | {
      state: "DENIED" | "RESTRICTED" | "UNAVAILABLE";
      action: "FALLBACK";
      reason:
        | "PERMISSION_DENIED"
        | "OS_RESTRICTED"
        | "CAPABILITY_UNAVAILABLE";
    };

export function decideCapability(state: CapabilityState): CapabilityDecision {
  switch (state) {
    case "GRANTED":
      return { state, action: "CONTINUE" };
    case "NOT_REQUESTED":
      return { state, action: "REQUEST_PERMISSION" };
    case "UNKNOWN":
      return { state, action: "REFRESH_STATE" };
    case "DENIED":
      return { state, action: "FALLBACK", reason: "PERMISSION_DENIED" };
    case "RESTRICTED":
      return { state, action: "FALLBACK", reason: "OS_RESTRICTED" };
    case "UNAVAILABLE":
      return { state, action: "FALLBACK", reason: "CAPABILITY_UNAVAILABLE" };
  }
}
