export type IdentityRole =
  | "CLIENT"
  | "SPECIALIST"
  | "AUTHOR"
  | "STUDENT";

export type AuthorizationDecision = "ALLOW" | "DENY" | "REQUIRE_STEP_UP";

export type PaymentProviderCode = string & { readonly __kind: "PaymentProviderCode" };
export type PaymentMethodCode = string & { readonly __kind: "PaymentMethodCode" };
export type PaymentRailCode = string & { readonly __kind: "PaymentRailCode" };

export interface PaymentRoute {
  provider: PaymentProviderCode;
  method: PaymentMethodCode;
  rail: PaymentRailCode;
}

export interface ApiReason {
  code: string;
  contractVersion: string;
}
