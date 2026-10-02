import type { CanonicalAnalyticsEventV1 } from "../../../packages/contracts/src/r1-mobile";
import {
  type NativeSurface,
  withNativeAnalyticsDiagnostics,
} from "./r1-cross-surface.ts";

export type AnalyticsTransport = (
  event: CanonicalAnalyticsEventV1,
) => void | Promise<void>;

export async function sendNativeAnalyticsEvent(
  event: Omit<CanonicalAnalyticsEventV1, "platform_extensions">,
  platform: NativeSurface,
  diagnostics: Record<string, string | number | boolean | null>,
  transport: AnalyticsTransport,
): Promise<CanonicalAnalyticsEventV1> {
  const canonical = withNativeAnalyticsDiagnostics(event, platform, diagnostics);
  await transport(canonical);
  return canonical;
}
