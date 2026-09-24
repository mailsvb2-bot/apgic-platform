import assert from "node:assert/strict";
import test from "node:test";

import {
  applyMutationServerAck,
  canonicalNotificationBusinessState,
  markMutationSubmitted,
  notificationDeliveryBelongsToIntent,
  queueOfflineMutation,
} from "./r2-transaction-edge.ts";

const envelope = {
  contract_version: "offline-mutation-v1" as const,
  identity_id: "identity-1",
  operation: "BOOKING_CONFIRM",
  idempotency_key: "mutation-1",
  request_digest: "sha256:payload-a",
};

test("offline mutation remains pending until server acknowledgement", () => {
  const queued = queueOfflineMutation(envelope);
  assert.equal(queued.state, "QUEUED_LOCAL");

  const submitted = markMutationSubmitted(queued);
  assert.equal(submitted.state, "AWAITING_SERVER");
  assert.equal(submitted.side_effect_ref, undefined);

  const acknowledged = applyMutationServerAck(submitted, {
    outcome: "APPLIED",
    side_effect_ref: "booking/booking-1",
    reason_code: "MUTATION_APPLIED",
  });
  assert.equal(acknowledged.state, "ACKNOWLEDGED");
  assert.equal(acknowledged.side_effect_ref, "booking/booking-1");
});

test("server conflict is never converted into local success", () => {
  const submitted = markMutationSubmitted(queueOfflineMutation(envelope));
  const conflicted = applyMutationServerAck(submitted, {
    outcome: "CONFLICT",
    reason_code: "MUTATION_IDEMPOTENCY_CONFLICT",
  });
  assert.equal(conflicted.state, "CONFLICT");
  assert.equal(conflicted.side_effect_ref, undefined);
});

test("push and email remain transports of one canonical intent", () => {
  const intent = {
    contract_version: "notification-intent-v1" as const,
    intent_id: "intent-1",
    booking_id: "booking-1",
    purpose: "BOOKING_CONFIRMATION",
    idempotency_key: "booking-1:confirmation",
    transactional: true as const,
    data_class: "SENSITIVE",
  };
  const deliveries = [
    {
      intent_id: "intent-1",
      channel: "PUSH" as const,
      delivery_idempotency_key: "intent-1:PUSH:device-1",
      state: "DELIVERED" as const,
    },
    {
      intent_id: "intent-1",
      channel: "EMAIL" as const,
      delivery_idempotency_key: "intent-1:EMAIL:mail-1",
      state: "SENT" as const,
    },
  ];

  assert.equal(notificationDeliveryBelongsToIntent(intent, deliveries[0]), true);
  assert.deepEqual(canonicalNotificationBusinessState(intent, deliveries), {
    intent_id: "intent-1",
    transactional: true,
    delivery_count: 2,
  });
});
