import type { PaymentRailCode } from "./foundation";

export type StoreClientSurface = "IOS" | "ANDROID";

export type StoreCommerceDecisionV1 =
  | {
      contract_version: "store-commerce-v1";
      policy_version: string;
      outcome: "ALLOWED";
      rail: PaymentRailCode;
      reason_code: string;
    }
  | {
      contract_version: "store-commerce-v1";
      policy_version: string;
      outcome: "PURCHASE_DISABLED";
      rail: "PURCHASE_DISABLED";
      reason_code: string;
    };

export interface StorePurchaseVerificationV1 {
  contract_version: "store-purchase-verification-v1";
  verification_id: string;
  order_id: string;
  payment_attempt_id: string;
  payment_effect_id: string;
  ledger_entry_id: string;
  external_transaction_id: string;
  provider_evidence_ref: string;
  server_verified: true;
}

export interface StoreEntitlementV1 {
  contract_version: "store-entitlement-v1";
  entitlement_id: string;
  identity_id: string;
  product_ref: string;
  kind: "ONE_TIME" | "SUBSCRIPTION";
  state: "ACTIVE" | "EXPIRED" | "REVOKED";
  verification_id: string;
}

export interface StoreSubscriptionExitV1 {
  contract_version: "store-subscription-exit-v1";
  delete_request_id: string;
  entitlement_id: string;
  action: "MANAGE_EXTERNALLY" | "NOT_APPLICABLE";
  management_target?: string;
  billing_cancelled_by_apgic: false;
  reason_code: string;
}

export type IntegrityVerdictV1 =
  | "VALID"
  | "NEGATIVE"
  | "UNSUPPORTED"
  | "UNAVAILABLE";

export interface DeviceIntegrityEvidenceV1 {
  contract_version: "device-integrity-v1";
  evidence_id: string;
  installation_id: string;
  verdict: IntegrityVerdictV1;
  server_verified: boolean;
  reason_code: string;
  policy_version: string;
  appeal_path: string;
  action: "ALLOW" | "STEP_UP" | "REVIEW";
}

export interface NonCashEntitlementEntryV1 {
  contract_version: "noncash-entitlement-v1";
  entry_id: string;
  account_ref: string;
  unit_kind: "CREDITS" | "ORGANIZATION_BUDGET" | "GROWTH_BUDGET";
  event_kind: "GRANT" | "SPEND" | "EXPIRE";
  units: number;
  source_ref: string;
  policy_version: string;
}
