import assert from "node:assert/strict";
import test from "node:test";

import { decideCapability } from "./device-capability.ts";

test("permission lifecycle distinguishes not requested from denied", () => {
  assert.deepEqual(decideCapability("NOT_REQUESTED"), {
    state: "NOT_REQUESTED",
    action: "REQUEST_PERMISSION",
  });
  assert.deepEqual(decideCapability("DENIED"), {
    state: "DENIED",
    action: "FALLBACK",
    reason: "PERMISSION_DENIED",
  });
});

test("unknown capability state requests refresh instead of guessing", () => {
  assert.deepEqual(decideCapability("UNKNOWN"), {
    state: "UNKNOWN",
    action: "REFRESH_STATE",
  });
});

test("unavailable capability never mutates server truth locally", () => {
  assert.deepEqual(decideCapability("UNAVAILABLE"), {
    state: "UNAVAILABLE",
    action: "FALLBACK",
    reason: "CAPABILITY_UNAVAILABLE",
  });
});
