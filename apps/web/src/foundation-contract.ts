import type {
  ApiReason,
  AuthorizationDecision,
  IdentityRole,
  PaymentRoute,
} from "../../../packages/contracts/src/foundation";

export interface WebFoundationSnapshot {
  identityId: string;
  roles: IdentityRole[];
  authorization: AuthorizationDecision;
  paymentRoute?: PaymentRoute;
  reason?: ApiReason;
}
