export type StoreLifecycleEventTypeV1 =
  | "RENEWAL"
  | "REFUND"
  | "REVOCATION"
  | "CHARGEBACK"
  | "GRACE_STARTED"
  | "HOLD_STARTED"
  | "EXPIRED";

export type StoreSubscriptionStateV1 =
  | "ACTIVE"
  | "GRACE"
  | "HOLD"
  | "EXPIRED"
  | "REVOKED";

export interface StoreLifecycleEventV1 {
  contract_version: "store-lifecycle-v1";
  provider_event_id: string;
  provider_instance_id: string;
  external_transaction_id: string;
  subscription_ref: string;
  sequence: number;
  event_type: StoreLifecycleEventTypeV1;
  provider_evidence_ref: string;
  ledger_evidence_ref?: string;
  occurred_at: string;
}

export interface StoreLifecycleProjectionV1 {
  contract_version: "store-lifecycle-projection-v1";
  provider_instance_id: string;
  subscription_ref: string;
  state: StoreSubscriptionStateV1;
  entitlement_state: "ACTIVE" | "EXPIRED" | "REVOKED";
  last_sequence: number;
  applied_event_count: number;
}
