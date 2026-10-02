import assert from "node:assert/strict";
import test from "node:test";

import { sendWebAnalyticsEvent } from "./analytics-client.ts";
import { withWebAnalyticsDiagnostics } from "./r1-cross-surface.ts";

const baseEvent = {
  event_name: "workspace_switched" as const,
  event_version: "1" as const,
  occurred_at: "2026-10-02T10:00:00Z",
  journey_id: "journey-analytics-1",
  identity_ref: "identity-1",
  business_properties: {
    workspace_id: "workspace-1",
    workspace_kind: "SPECIALIST",
  },
};

test("web analytics keeps diagnostics namespaced from business semantics", () => {
  const event = withWebAnalyticsDiagnostics(baseEvent, {
    viewport_class: "DESKTOP",
  });
  assert.deepEqual(event.business_properties, baseEvent.business_properties);
  assert.deepEqual(event.platform_extensions, {
    WEB: { viewport_class: "DESKTOP" },
  });
});

test("web analytics sender emits exactly the canonical event it returns", async () => {
  const delivered: unknown[] = [];
  const event = await sendWebAnalyticsEvent(
    baseEvent,
    { viewport_class: "DESKTOP" },
    (candidate) => {
      delivered.push(candidate);
    },
  );
  assert.equal(delivered.length, 1);
  assert.deepEqual(delivered[0], event);
});
