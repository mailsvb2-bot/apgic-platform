import type {
  MutationServerAckV1,
  NotificationDeliveryV1,
  OfflineMutationEnvelopeV1,
  OfflineMutationViewV1,
  TransactionalNotificationIntentV1,
} from "../../../packages/contracts/src/r2-transaction-edge";

export function queueOfflineMutation(
  envelope: OfflineMutationEnvelopeV1,
): OfflineMutationViewV1 {
  return { envelope, state: "QUEUED_LOCAL" };
}

export function markMutationSubmitted(
  view: OfflineMutationViewV1,
): OfflineMutationViewV1 {
  if (view.state !== "QUEUED_LOCAL") {
    return view;
  }
  return { ...view, state: "AWAITING_SERVER" };
}

export function applyMutationServerAck(
  view: OfflineMutationViewV1,
  ack: MutationServerAckV1,
): OfflineMutationViewV1 {
  if (ack.outcome === "CONFLICT") {
    return {
      ...view,
      state: "CONFLICT",
      reason_code: ack.reason_code,
    };
  }
  if (!ack.side_effect_ref) {
    return {
      ...view,
      state: "CONFLICT",
      reason_code: "MUTATION_ACK_MISSING_SIDE_EFFECT",
    };
  }
  return {
    ...view,
    state: "ACKNOWLEDGED",
    side_effect_ref: ack.side_effect_ref,
    reason_code: ack.reason_code,
  };
}

export function notificationDeliveryBelongsToIntent(
  intent: TransactionalNotificationIntentV1,
  delivery: NotificationDeliveryV1,
): boolean {
  return intent.intent_id === delivery.intent_id;
}

export function canonicalNotificationBusinessState(
  intent: TransactionalNotificationIntentV1,
  deliveries: readonly NotificationDeliveryV1[],
): { intent_id: string; transactional: true; delivery_count: number } {
  const relevant = deliveries.filter((item) => item.intent_id === intent.intent_id);
  return {
    intent_id: intent.intent_id,
    transactional: true,
    delivery_count: relevant.length,
  };
}
