import assert from "node:assert/strict";
import test from "node:test";

import { sendNativeAnalyticsEvent } from "./analytics-client.ts";

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

for (const platform of ["IOS", "ANDROID"] as const) {
  test(`${platform} analytics sender keeps business semantics canonical`, async () => {
    const delivered: unknown[] = [];
    const event = await sendNativeAnalyticsEvent(
      baseEvent,
      platform,
      { app_version: "1.0.0" },
      (candidate) => {
        delivered.push(candidate);
      },
    );
    assert.deepEqual(event.business_properties, baseEvent.business_properties);
    assert.deepEqual(event.platform_extensions, {
      [platform]: { app_version: "1.0.0" },
    });
    assert.deepEqual(delivered, [event]);
  });
}
