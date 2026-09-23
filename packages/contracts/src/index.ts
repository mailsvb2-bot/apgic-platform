export type Surface = "WEB" | "PWA" | "IOS" | "ANDROID";

export type ErrorEnvelope = {
  code: string;
  message_safe: string;
  correlation_id: string;
  retryable: boolean;
  field_errors?: Array<Record<string, unknown>>;
  policy_reason_codes?: string[];
  documentation_ref?: string;
};

export type PaymentSelection = {
  provider_id: string;
  method_code: string;
  rail_code: string;
};

export const releaseTrack = "R0" as const;
