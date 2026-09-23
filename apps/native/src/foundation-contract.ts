import type {
  ApiReason,
  AuthorizationDecision,
  IdentityRole,
  PaymentRoute,
} from "../../../packages/contracts/src/foundation";

export interface NativeFoundationSnapshot {
  identityId: string;
  roles: IdentityRole[];
  authorization: AuthorizationDecision;
  paymentRoute?: PaymentRoute;
  reason?: ApiReason;
}

export type NativePlatform = "IOS" | "ANDROID";
