export type DeviceCapability =
  | "CAMERA"
  | "MICROPHONE"
  | "CALENDAR"
  | "SECURE_STORAGE"
  | "PUSH_NOTIFICATIONS";

export type CapabilityState =
  | "AVAILABLE"
  | "DENIED"
  | "RESTRICTED"
  | "UNAVAILABLE";

export type CapabilityDecision =
  | { state: "AVAILABLE"; action: "CONTINUE" }
  | {
      state: Exclude<CapabilityState, "AVAILABLE">;
      action: "FALLBACK";
      reason:
        | "PERMISSION_DENIED"
        | "OS_RESTRICTED"
        | "CAPABILITY_UNAVAILABLE";
    };

export function decideCapability(state: CapabilityState): CapabilityDecision {
  switch (state) {
    case "AVAILABLE":
      return { state, action: "CONTINUE" };
    case "DENIED":
      return { state, action: "FALLBACK", reason: "PERMISSION_DENIED" };
    case "RESTRICTED":
      return { state, action: "FALLBACK", reason: "OS_RESTRICTED" };
    case "UNAVAILABLE":
      return { state, action: "FALLBACK", reason: "CAPABILITY_UNAVAILABLE" };
  }
}
