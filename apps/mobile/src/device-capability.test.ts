import assert from "node:assert/strict";
import test from "node:test";

import { decideCapability } from "./device-capability.ts";

test("denied microphone permission produces deterministic fallback", () => {
  assert.deepEqual(decideCapability("DENIED"), {
    state: "DENIED",
    action: "FALLBACK",
    reason: "PERMISSION_DENIED",
  });
});

test("unavailable capability never mutates server truth locally", () => {
  assert.deepEqual(decideCapability("UNAVAILABLE"), {
    state: "UNAVAILABLE",
    action: "FALLBACK",
    reason: "CAPABILITY_UNAVAILABLE",
  });
});
