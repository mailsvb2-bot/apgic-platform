import assert from "node:assert/strict";
import test from "node:test";

import {
  motionMode,
  validateCriticalControl,
} from "./accessibility-policy.ts";

const policy = {
  policy_version: "mobile-support-test-v1",
  minimum_ios_major: 16,
  minimum_android_api: 29,
  accessibility: {
    voice_over: true,
    talk_back: true,
    text_scaling: true,
    reduced_motion: true,
    focus_order: true,
    touch_target_min_dp: 44,
  },
} as const;

test("critical control requirements come from versioned support policy", () => {
  assert.deepEqual(
    validateCriticalControl(policy, {
      label: "Удалить аккаунт",
      role: "button",
      touchTargetDp: 44,
      focusable: true,
    }),
    [],
  );

  assert.deepEqual(
    validateCriticalControl(policy, {
      label: "",
      role: "",
      touchTargetDp: 32,
      focusable: false,
    }),
    [
      "LABEL_REQUIRED",
      "ROLE_REQUIRED",
      "FOCUS_REQUIRED",
      "TOUCH_TARGET_BELOW_POLICY",
    ],
  );
});

test("reduced motion follows OS preference when policy supports it", () => {
  assert.equal(motionMode(policy, true), "REDUCED");
  assert.equal(motionMode(policy, false), "STANDARD");
});
