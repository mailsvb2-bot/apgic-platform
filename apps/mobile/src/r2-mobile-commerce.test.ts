import assert from "node:assert/strict";
import test from "node:test";

import type {
  DeviceIntegrityEvidenceV1,
  StoreCommerceDecisionV1,
  StoreEntitlementV1,
  StorePurchaseVerificationV1,
  StoreSubscriptionExitV1,
} from "../../../packages/contracts/src/r2-mobile-commerce";
import {
  acceptServerVerifiedEntitlement,
  consumeIntegrityEvidence,
  consumeStoreCommerceDecision,
  presentStoreSubscriptionExit,
} from "./r2-mobile-commerce.ts";

test("native store flow consumes server rail decision instead of platform hardcoding", () => {
  const decision = {
    contract_version: "store-commerce-v1",
    policy_version: "store-policy-v1",
    outcome: "PURCHASE_DISABLED",
    rail: "PURCHASE_DISABLED",
    reason_code: "STORE_POLICY_NOT_CONFIGURED",
  } satisfies StoreCommerceDecisionV1;

  assert.deepEqual(consumeStoreCommerceDecision(decision), {
    action: "PURCHASE_DISABLED",
    reasonCode: "STORE_POLICY_NOT_CONFIGURED",
    policyVersion: "store-policy-v1",
  });
});

test("local store receipt cannot activate entitlement without server verification", () => {
  const verification = {
    contract_version: "store-purchase-verification-v1",
    verification_id: "verification-1",
    order_id: "order-1",
    payment_attempt_id: "payment-1",
    payment_effect_id: "effect-1",
    ledger_entry_id: "ledger-1",
    external_transaction_id: "store-tx-1",
    provider_evidence_ref: "provider-evidence/1",
    server_verified: true,
  } satisfies StorePurchaseVerificationV1;
  const entitlement = {
    contract_version: "store-entitlement-v1",
    entitlement_id: "entitlement-1",
    identity_id: "identity-1",
    product_ref: "product/subscription-1",
    kind: "SUBSCRIPTION",
    state: "ACTIVE",
    verification_id: "verification-1",
  } satisfies StoreEntitlementV1;

  assert.deepEqual(
    acceptServerVerifiedEntitlement(verification, entitlement),
    entitlement,
  );
  assert.equal(
    acceptServerVerifiedEntitlement(
      verification,
      { ...entitlement, verification_id: "another-verification" },
    ),
    null,
  );
});

test("account deletion never claims APGIC cancelled external store billing", () => {
  const exit = {
    contract_version: "store-subscription-exit-v1",
    delete_request_id: "delete-1",
    entitlement_id: "entitlement-1",
    action: "MANAGE_EXTERNALLY",
    management_target: "https://store.example/subscriptions",
    billing_cancelled_by_apgic: false,
    reason_code: "EXTERNAL_STORE_SUBSCRIPTION_ACTIVE",
  } satisfies StoreSubscriptionExitV1;

  assert.deepEqual(presentStoreSubscriptionExit(exit), {
    action: "OPEN_EXTERNAL_MANAGEMENT",
    target: "https://store.example/subscriptions",
    messageKey: "STORE_SUBSCRIPTION_MANAGE_EXTERNALLY",
  });
});

test("negative integrity evidence becomes step-up, not local ban", () => {
  const evidence = {
    contract_version: "device-integrity-v1",
    evidence_id: "integrity-1",
    installation_id: "installation-1",
    verdict: "NEGATIVE",
    server_verified: true,
    reason_code: "DEVICE_INTEGRITY_NEGATIVE_STEP_UP",
    policy_version: "integrity-v1",
    appeal_path: "/support/integrity-appeal",
    action: "STEP_UP",
  } satisfies DeviceIntegrityEvidenceV1;

  assert.deepEqual(consumeIntegrityEvidence(evidence), {
    action: "STEP_UP",
    reasonCode: "DEVICE_INTEGRITY_NEGATIVE_STEP_UP",
    appealPath: "/support/integrity-appeal",
  });
});


test("unverified negative integrity signal cannot become trusted native decision", () => {
  const evidence = {
    contract_version: "device-integrity-v1",
    evidence_id: "integrity-unverified",
    installation_id: "installation-1",
    verdict: "NEGATIVE",
    server_verified: false,
    reason_code: "LOCAL_SIGNAL_ONLY",
    policy_version: "integrity-v1",
    appeal_path: "/support/integrity-appeal",
    action: "STEP_UP",
  } satisfies DeviceIntegrityEvidenceV1;

  assert.deepEqual(consumeIntegrityEvidence(evidence), {
    action: "REVIEW",
    reasonCode: "DEVICE_INTEGRITY_NOT_SERVER_VERIFIED",
    appealPath: "/support/integrity-appeal",
  });
});
