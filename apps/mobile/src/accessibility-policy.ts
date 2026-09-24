import type { MobileSupportPolicyV1 } from "../../../packages/contracts/src/r1-mobile";

export interface CriticalControlAccessibility {
  label: string;
  role: string;
  touchTargetDp: number;
  focusable: boolean;
}

export type AccessibilityIssue =
  | "LABEL_REQUIRED"
  | "ROLE_REQUIRED"
  | "FOCUS_REQUIRED"
  | "TOUCH_TARGET_BELOW_POLICY";

export function validateCriticalControl(
  policy: MobileSupportPolicyV1,
  control: CriticalControlAccessibility,
): AccessibilityIssue[] {
  const issues: AccessibilityIssue[] = [];
  if (!control.label.trim()) {
    issues.push("LABEL_REQUIRED");
  }
  if (!control.role.trim()) {
    issues.push("ROLE_REQUIRED");
  }
  if (!control.focusable) {
    issues.push("FOCUS_REQUIRED");
  }
  if (control.touchTargetDp < policy.accessibility.touch_target_min_dp) {
    issues.push("TOUCH_TARGET_BELOW_POLICY");
  }
  return issues;
}

export function motionMode(
  policy: MobileSupportPolicyV1,
  osReducedMotionEnabled: boolean,
): "REDUCED" | "STANDARD" {
  if (policy.accessibility.reduced_motion && osReducedMotionEnabled) {
    return "REDUCED";
  }
  return "STANDARD";
}
