import assert from "node:assert/strict";
import { sendWebAnalyticsEvent } from "../apps/web/src/analytics-client.ts";
import { sendNativeAnalyticsEvent } from "../apps/mobile/src/analytics-client.ts";

const base = Object.freeze({
  event_name: "workspace_switched",
  event_version: "1",
  occurred_at: "2026-10-02T10:00:00Z",
  journey_id: "journey-analytics-parity",
  identity_ref: "identity-analytics",
  business_properties: Object.freeze({
    workspace_id: "workspace-analytics",
    workspace_kind: "SPECIALIST",
  }),
});

const deliveries = [];
const transport = async (event) => {
  deliveries.push(structuredClone(event));
};

const web = await sendWebAnalyticsEvent(
  base,
  { viewport_class: "DESKTOP", renderer: "NEXTJS" },
  transport,
);
const ios = await sendNativeAnalyticsEvent(
  base,
  "IOS",
  { app_version: "1.0.0", os_family: "IOS" },
  transport,
);
const android = await sendNativeAnalyticsEvent(
  base,
  "ANDROID",
  { app_version: "1.0.0", os_family: "ANDROID" },
  transport,
);

assert.equal(deliveries.length, 3);

const canonicalCore = (event) => {
  const { platform_extensions: _extensions, ...core } = event;
  return core;
};

assert.deepEqual(canonicalCore(web), canonicalCore(ios));
assert.deepEqual(canonicalCore(web), canonicalCore(android));
assert.deepEqual(Object.keys(web.platform_extensions), ["WEB"]);
assert.deepEqual(Object.keys(ios.platform_extensions), ["IOS"]);
assert.deepEqual(Object.keys(android.platform_extensions), ["ANDROID"]);
assert.equal(web.event_name, "workspace_switched");
assert.equal(ios.event_name, web.event_name);
assert.equal(android.event_name, web.event_name);
assert.deepEqual(web.business_properties, ios.business_properties);
assert.deepEqual(web.business_properties, android.business_properties);

console.log(
  "R1 ANALYTICS PARITY E2E: PASS (WEB/IOS/ANDROID canonical business semantics match; diagnostics remain namespaced)",
);
