export type TransactionalNotificationIntentV1 = {
  contract_version: "notification-intent-v1";
  intent_id: string;
  booking_id: string;
  purpose: string;
  idempotency_key: string;
  transactional: true;
  data_class: string;
};

export type NotificationChannelV1 = "PUSH" | "EMAIL" | "SMS";

export type NotificationDeliveryV1 = {
  intent_id: string;
  channel: NotificationChannelV1;
  delivery_idempotency_key: string;
  state: "PENDING" | "SENT" | "DELIVERED" | "FAILED_RETRYABLE" | "SUPPRESSED";
};

export type OfflineMutationStateV1 =
  | "QUEUED_LOCAL"
  | "AWAITING_SERVER"
  | "ACKNOWLEDGED"
  | "CONFLICT";

export type OfflineMutationEnvelopeV1 = {
  contract_version: "offline-mutation-v1";
  identity_id: string;
  operation: string;
  idempotency_key: string;
  request_digest: string;
};

export type OfflineMutationViewV1 = {
  envelope: OfflineMutationEnvelopeV1;
  state: OfflineMutationStateV1;
  side_effect_ref?: string;
  reason_code?: string;
};

export type MutationServerAckV1 = {
  outcome: "APPLIED" | "DUPLICATE_APPLIED" | "CONFLICT";
  side_effect_ref?: string;
  reason_code: string;
};

export type CalendarSyncProjectionV1 = {
  contract_version: "calendar-sync-v1";
  booking_id: string;
  booking_version: number;
  provider_instance_id: string;
  target_ref: string;
  idempotency_key: string;
  state: "PENDING" | "SYNCED" | "CONFLICT" | "FAILED_RETRYABLE";
  provider_event_ref?: string;
};
