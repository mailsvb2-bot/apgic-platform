import assert from "node:assert/strict";
import test from "node:test";
import nacl from "tweetnacl";

import type {
  RemoteCapability,
  SignedRemoteConfigEnvelope,
} from "../../../packages/contracts/src/mobile-policy.ts";
import {
  canonicalRemoteConfigPayload,
  isSignedRemoteConfigEnvelope,
  resolveRemoteConfig,
  verifySignedRemoteConfigEnvelope,
  type RemoteConfigStorage,
} from "./remote-config-client.ts";

const now = new Date("2026-10-02T20:30:00Z");
const signingSeed = Uint8Array.from({length: 32}, (_, index) => index + 1);
const signingKey = nacl.sign.keyPair.fromSeed(signingSeed);
const keyID = "mobile027-test-key";
const trustedKeys = {
  [keyID]: Buffer.from(signingKey.publicKey).toString("base64"),
};

function envelope(
  version: number,
  disabled: RemoteCapability[] = [],
  expiresAt = "2026-10-02T21:00:00Z",
): SignedRemoteConfigEnvelope {
  const payload = {
    version,
    issued_at: "2026-10-02T20:00:00Z",
    expires_at: expiresAt,
    policy_id: "mobile027-policy-v1",
    disabled_capabilities: disabled,
    reason_codes: disabled.length
      ? Object.fromEntries(
          disabled.map((capability) => [capability, "INCIDENT_DISABLE"]),
        )
      : undefined,
  };
  const signature = nacl.sign.detached(
    new TextEncoder().encode(canonicalRemoteConfigPayload(payload)),
    signingKey.secretKey,
  );
  return {
    key_id: keyID,
    payload,
    signature: Buffer.from(signature).toString("base64"),
  };
}

function memoryStorage(initial: string | null = null) {
  let value = initial;
  const storage: RemoteConfigStorage = {
    async load() { return value; },
    async save(next) { value = next; },
    async clear() { value = null; },
  };
  return {storage, read: () => value};
}

test("accepts signed versioned remote config shape and verifies Ed25519", () => {
  const signed = envelope(2, ["REALTIME_CONSULTATION"]);
  assert.equal(isSignedRemoteConfigEnvelope(signed, now), true);
  assert.equal(
    verifySignedRemoteConfigEnvelope(signed, trustedKeys, now),
    true,
  );

  const tampered = structuredClone(signed);
  tampered.payload.disabled_capabilities = [];
  assert.equal(
    verifySignedRemoteConfigEnvelope(tampered, trustedKeys, now),
    false,
  );
});

test("rejects expired signed config before capability activation", () => {
  const expired = envelope(2);
  expired.payload.expires_at = "2026-10-02T20:29:59Z";
  assert.equal(isSignedRemoteConfigEnvelope(expired, now), false);
  assert.equal(
    verifySignedRemoteConfigEnvelope(expired, trustedKeys, now),
    false,
  );
});

test("uses network config and persists it as last-known-safe", async () => {
  const originalFetch = globalThis.fetch;
  const memory = memoryStorage();
  globalThis.fetch = (async () =>
    new Response(JSON.stringify(envelope(3, ["REALTIME_CONSULTATION"])), {
      status: 200,
      headers: {"content-type": "application/json"},
    })) as typeof fetch;
  try {
    const result = await resolveRemoteConfig({
      baseURL: "https://apgic.ru",
      storage: memory.storage,
      trustedKeys,
      now,
    });
    assert.equal(result.source, "NETWORK");
    assert.deepEqual(result.disabledCapabilities, ["REALTIME_CONSULTATION"]);
    assert.match(memory.read() ?? "", /mobile027-policy-v1/);
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("network failure keeps unexpired last-known-safe signed config", async () => {
  const originalFetch = globalThis.fetch;
  const memory = memoryStorage(JSON.stringify(envelope(4, ["PERSONA_PREVIEW"])));
  globalThis.fetch = (async () => {
    throw new Error("network down");
  }) as typeof fetch;
  try {
    const result = await resolveRemoteConfig({
      baseURL: "https://apgic.ru",
      storage: memory.storage,
      trustedKeys,
      now,
    });
    assert.equal(result.source, "LAST_KNOWN_SAFE");
    assert.deepEqual(result.disabledCapabilities, ["PERSONA_PREVIEW"]);
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("rejects remote rollback and keeps newer signed cached version", async () => {
  const originalFetch = globalThis.fetch;
  const memory = memoryStorage(JSON.stringify(envelope(9, ["CALENDAR_INTEGRATION"])));
  globalThis.fetch = (async () =>
    new Response(JSON.stringify(envelope(8)), {status: 200})) as typeof fetch;
  try {
    const result = await resolveRemoteConfig({
      baseURL: "https://apgic.ru",
      storage: memory.storage,
      trustedKeys,
      now,
    });
    assert.equal(result.source, "LAST_KNOWN_SAFE");
    assert.equal(result.reasonCode, "REMOTE_CONFIG_ROLLBACK_REJECTED");
    assert.deepEqual(result.disabledCapabilities, ["CALENDAR_INTEGRATION"]);
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("applies verified network kill switch even when persistence fails", async () => {
  const originalFetch = globalThis.fetch;
  const storage: RemoteConfigStorage = {
    async load() { return JSON.stringify(envelope(2)); },
    async save() { throw new Error("disk full"); },
    async clear() {},
  };
  globalThis.fetch = (async () =>
    new Response(JSON.stringify(envelope(3, ["REALTIME_CONSULTATION"])), {
      status: 200,
      headers: {"content-type": "application/json"},
    })) as typeof fetch;
  try {
    const result = await resolveRemoteConfig({
      baseURL: "https://apgic.ru",
      storage,
      trustedKeys,
      now,
    });
    assert.equal(result.source, "NETWORK");
    assert.equal(result.reasonCode, "REMOTE_CONFIG_APPLIED_PERSISTENCE_FAILED");
    assert.deepEqual(result.disabledCapabilities, ["REALTIME_CONSULTATION"]);
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("expired signed cache remains an anti-rollback high-water mark", async () => {
  const originalFetch = globalThis.fetch;
  const expired = envelope(9, ["CALENDAR_INTEGRATION"], "2026-10-02T20:29:59Z");
  const memory = memoryStorage(JSON.stringify(expired));
  globalThis.fetch = (async () =>
    new Response(JSON.stringify(envelope(8)), {
      status: 200,
      headers: {"content-type": "application/json"},
    })) as typeof fetch;
  try {
    const result = await resolveRemoteConfig({
      baseURL: "https://apgic.ru",
      storage: memory.storage,
      trustedKeys,
      now,
    });
    assert.equal(result.source, "FAIL_SAFE");
    assert.equal(result.reasonCode, "REMOTE_CONFIG_ROLLBACK_REJECTED");
    assert.deepEqual(result.disabledCapabilities.sort(), [
      "CALENDAR_INTEGRATION",
      "PERSONA_PREVIEW",
      "REALTIME_CONSULTATION",
    ]);
    assert.equal(memory.read(), JSON.stringify(expired));
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("missing network and cache fails safe with all remote capabilities disabled", async () => {
  const originalFetch = globalThis.fetch;
  const memory = memoryStorage();
  globalThis.fetch = (async () => {
    throw new Error("network down");
  }) as typeof fetch;
  try {
    const result = await resolveRemoteConfig({
      baseURL: "https://apgic.ru",
      storage: memory.storage,
      trustedKeys,
      now,
    });
    assert.equal(result.source, "FAIL_SAFE");
    assert.deepEqual(result.disabledCapabilities.sort(), [
      "CALENDAR_INTEGRATION",
      "PERSONA_PREVIEW",
      "REALTIME_CONSULTATION",
    ]);
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("unknown signing key fails safe instead of trusting server-provided identity", async () => {
  const originalFetch = globalThis.fetch;
  const memory = memoryStorage();
  globalThis.fetch = (async () =>
    new Response(JSON.stringify(envelope(1)), {status: 200})) as typeof fetch;
  try {
    const result = await resolveRemoteConfig({
      baseURL: "https://apgic.ru",
      storage: memory.storage,
      trustedKeys: {},
      now,
    });
    assert.equal(result.source, "FAIL_SAFE");
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("privileged business truth cannot appear as a remote capability", () => {
  const signed = envelope(1);
  signed.payload.disabled_capabilities = [
    "ENTITLEMENT_GRANT" as RemoteCapability,
  ];
  assert.equal(isSignedRemoteConfigEnvelope(signed, now), false);
});
