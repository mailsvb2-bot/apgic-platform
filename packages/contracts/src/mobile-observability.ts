export type MobileMetricName =
  | "crash_rate"
  | "anr_rate"
  | "startup_p95_ms"
  | "push_delivery_success"
  | "deep_link_success"
  | "checkout_success"
  | "realtime_join_success"
  | "reconnect_success";

export type MobileMetricUnit = "ratio" | "milliseconds";

export interface MobileMetricEnvelope {
  metric: MobileMetricName;
  value: number;
  unit: MobileMetricUnit;
  app_version: string;
  platform: "IOS" | "ANDROID";
  os_version: string;
  device_class: string;
  network_class: string;
  release_channel: string;
  policy_version: string;
  occurred_at: string;
}

/**
 * Deliberately no arbitrary payload/attributes field.
 * Raw consultation/persona content, credentials and direct personal identifiers
 * are outside this telemetry contract.
 */
