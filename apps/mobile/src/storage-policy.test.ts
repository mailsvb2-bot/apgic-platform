import assert from "node:assert/strict";
import test from "node:test";

import { canPersistLocally } from "./storage-policy.ts";

test("credentials require secure storage", () => {
  assert.equal(canPersistLocally("CREDENTIAL", "SECURE_STORAGE"), true);
  assert.equal(canPersistLocally("CREDENTIAL", "PERSISTENT_CACHE"), false);
});

test("raw sensitive data is not allowed in persistent cache", () => {
  for (const dataClass of [
    "RAW_CONSULTATION",
    "RAW_PERSONA",
    "FINANCIAL_EVIDENCE",
  ] as const) {
    assert.equal(canPersistLocally(dataClass, "PERSISTENT_CACHE"), false);
    assert.equal(canPersistLocally(dataClass, "EPHEMERAL_CACHE"), true);
  }
});
