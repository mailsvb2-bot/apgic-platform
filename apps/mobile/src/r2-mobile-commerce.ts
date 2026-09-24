import type {
  DeviceIntegrityEvidenceV1,
  StoreCommerceDecisionV1,
  StoreEntitlementV1,
  StorePurchaseVerificationV1,
  StoreSubscriptionExitV1,
} from "../../../packages/contracts/src/r2-mobile-commerce";

export type NativeStorePurchaseAction =
  | { action: "START_PROVIDER_PURCHASE"; rail: string; policyVersion: string }
  | { action: "PURCHASE_DISABLED"; reasonCode: string; policyVersion: string };

export function consumeStoreCommerceDecision(
  decision: StoreCommerceDecisionV1,
): NativeStorePurchaseAction {
  if (decision.outcome === "PURCHASE_DISABLED") {
    return {
      action: "PURCHASE_DISABLED",
      reasonCode: decision.reason_code,
      policyVersion: decision.policy_version,
    };
  }
  return {
    action: "START_PROVIDER_PURCHASE",
    rail: decision.rail as string,
    policyVersion: decision.policy_version,
  };
}

export function acceptServerVerifiedEntitlement(
  verification: StorePurchaseVerificationV1,
  entitlement: StoreEntitlementV1,
): StoreEntitlementV1 | null {
  if (
    verification.server_verified !== true ||
    entitlement.verification_id !== verification.verification_id ||
    entitlement.state !== "ACTIVE"
  ) {
    return null;
  }
  return entitlement;
}

export function presentStoreSubscriptionExit(
  exit: StoreSubscriptionExitV1,
):
  | { action: "OPEN_EXTERNAL_MANAGEMENT"; target: string; messageKey: string }
  | { action: "NO_EXTERNAL_ACTION"; messageKey: string } {
  if (
    exit.action === "MANAGE_EXTERNALLY" &&
    exit.management_target &&
    exit.billing_cancelled_by_apgic === false
  ) {
    return {
      action: "OPEN_EXTERNAL_MANAGEMENT",
      target: exit.management_target,
      messageKey: "STORE_SUBSCRIPTION_MANAGE_EXTERNALLY",
    };
  }
  return {
    action: "NO_EXTERNAL_ACTION",
    messageKey: "STORE_SUBSCRIPTION_NOT_APPLICABLE",
  };
}

export function consumeIntegrityEvidence(
  evidence: DeviceIntegrityEvidenceV1,
):
  | { action: "CONTINUE" }
  | { action: "STEP_UP"; reasonCode: string; appealPath: string }
  | { action: "REVIEW"; reasonCode: string; appealPath: string } {
  switch (evidence.action) {
    case "ALLOW":
      return { action: "CONTINUE" };
    case "STEP_UP":
      return {
        action: "STEP_UP",
        reasonCode: evidence.reason_code,
        appealPath: evidence.appeal_path,
      };
    case "REVIEW":
      return {
        action: "REVIEW",
        reasonCode: evidence.reason_code,
        appealPath: evidence.appeal_path,
      };
  }
}
