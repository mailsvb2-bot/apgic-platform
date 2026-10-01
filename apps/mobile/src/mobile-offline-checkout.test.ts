import assert from "node:assert/strict";
import test from "node:test";

import {
  createOfflineCheckoutQueueItem,
  loadOfflineCheckout,
  persistOfflineCheckout,
  requireOfflineCheckoutQueueItem,
  syncOfflineCheckout,
  type OfflineCheckoutFetch,
  type OfflineMutationStorage,
} from "./mobile-offline-checkout.ts";

class MemoryStorage implements OfflineMutationStorage {
  value: string | null = null;

  async load(): Promise<string | null> {
    return this.value;
  }

  async save(value: string): Promise<void> {
    this.value = value;
  }

  async clear(): Promise<void> {
    this.value = null;
  }
}

function queue(now = new Date("2026-10-01T12:00:00Z")) {
  return createOfflineCheckoutQueueItem({
    idempotencyKey: "checkout:hold-1:BANK_CARD",
    holdID: "hold-1",
    methodCode: "BANK_CARD",
    now,
    expiresAt: new Date(now.getTime() + 10 * 60_000),
    maxAttempts: 3,
  });
}

test("network failure remains visibly pending and survives restart", async () => {
  const storage = new MemoryStorage();
  const initial = queue();
  await persistOfflineCheckout(storage, initial);

  const offline: OfflineCheckoutFetch = async () => {
    throw new Error("network down");
  };
  const pending = await syncOfflineCheckout(
    initial,
    storage,
    {baseURL: "https://apgic.ru"},
    offline,
    new Date("2026-10-01T12:01:00Z"),
  );
  assert.equal(pending.state, "LOCAL_PENDING");
  assert.equal(pending.attempts, 1);
  assert.equal(pending.side_effect_ref, undefined);

  const afterRestart = await loadOfflineCheckout(storage);
  assert.ok(afterRestart);
  assert.equal(afterRestart.state, "LOCAL_PENDING");
  assert.equal(afterRestart.attempts, 1);

  const recovered: OfflineCheckoutFetch = async (_url, options) => {
    assert.equal(options?.headers?.["Idempotency-Key"], initial.idempotency_key);
    assert.equal(options?.headers?.["X-Correlation-Id"], initial.correlation_id);
    return {
      status: 200,
      async json() {
        return {
          contract_version: "offline-checkout-mutation-v1",
          outcome: "DUPLICATE_APPLIED",
          reason_code: "MUTATION_DUPLICATE_APPLIED",
          side_effect_ref: "checkout/instruction-1",
          checkout: {id: "instruction-1"},
        };
      },
    };
  };
  const confirmed = await syncOfflineCheckout(
    afterRestart,
    storage,
    {baseURL: "https://apgic.ru"},
    recovered,
    new Date("2026-10-01T12:02:00Z"),
  );
  assert.equal(confirmed.state, "SERVER_CONFIRMED");
  assert.equal(confirmed.side_effect_ref, "checkout/instruction-1");
  assert.equal(storage.value, null);
});

test("same idempotency key with changed server digest becomes conflict", async () => {
  const storage = new MemoryStorage();
  const item = queue();
  const conflict: OfflineCheckoutFetch = async () => ({
    status: 409,
    async json() {
      return {code: "MUTATION_IDEMPOTENCY_CONFLICT"};
    },
  });
  const result = await syncOfflineCheckout(
    item,
    storage,
    {baseURL: "https://apgic.ru"},
    conflict,
    new Date("2026-10-01T12:01:00Z"),
  );
  assert.equal(result.state, "CONFLICT");
  assert.equal(result.reason_code, "MUTATION_IDEMPOTENCY_CONFLICT");
  assert.ok(storage.value);
});

test("expiry and retry bound fail without a false server confirmation", async () => {
  const storage = new MemoryStorage();
  const expired = queue(new Date("2026-10-01T11:00:00Z"));
  let requests = 0;
  const request: OfflineCheckoutFetch = async () => {
    requests += 1;
    throw new Error("must not run");
  };
  const expiredResult = await syncOfflineCheckout(
    expired,
    storage,
    {baseURL: "https://apgic.ru"},
    request,
    new Date("2026-10-01T12:00:00Z"),
  );
  assert.equal(expiredResult.state, "FAILED");
  assert.equal(expiredResult.reason_code, "MUTATION_QUEUE_EXPIRED");
  assert.equal(requests, 0);

  const exhausted = {...queue(), attempts: 3};
  const exhaustedResult = await syncOfflineCheckout(
    exhausted,
    storage,
    {baseURL: "https://apgic.ru"},
    request,
    new Date("2026-10-01T12:01:00Z"),
  );
  assert.equal(exhaustedResult.state, "FAILED");
  assert.equal(exhaustedResult.reason_code, "MUTATION_RETRY_EXHAUSTED");
  assert.equal(requests, 0);
});

test("persistent queue rejects undeclared or sensitive payload fields", () => {
  const item = queue();
  assert.throws(
    () =>
      requireOfflineCheckoutQueueItem({
        ...item,
        consultation_text: "must never be persisted here",
      }),
    /OFFLINE_MUTATION_QUEUE_INVALID/,
  );
});
