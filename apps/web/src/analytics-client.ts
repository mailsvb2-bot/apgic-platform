import type { CanonicalAnalyticsEventV1 } from "../../../packages/contracts/src/r1-mobile";
import { withWebAnalyticsDiagnostics } from "./r1-cross-surface.ts";

export type AnalyticsTransport = (
  event: CanonicalAnalyticsEventV1,
) => void | Promise<void>;

export async function sendWebAnalyticsEvent(
  event: Omit<CanonicalAnalyticsEventV1, "platform_extensions">,
  diagnostics: Record<string, string | number | boolean | null>,
  transport: AnalyticsTransport,
): Promise<CanonicalAnalyticsEventV1> {
  const canonical = withWebAnalyticsDiagnostics(event, diagnostics);
  await transport(canonical);
  return canonical;
}
