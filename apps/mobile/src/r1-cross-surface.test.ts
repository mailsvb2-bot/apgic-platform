import assert from "node:assert/strict";
import test from "node:test";

import {
  buildNativeDeleteAccountInitiation,
  consumeAuthorizedWorkspace,
  resolveNativeDeepLink,
  withNativeAnalyticsDiagnostics,
} from "./r1-cross-surface.ts";

test("native account deletion uses the canonical cross-surface contract", () => {
  assert.deepEqual(
    buildNativeDeleteAccountInitiation("request-1", "identity-1", "IOS"),
    {
      contract_version: "delete-account-v1",
      request_id: "request-1",
      identity_id: "identity-1",
      source: "IOS",
    },
  );
});

test("native deep link never converts a server DENY into app authorization", () => {
  assert.deepEqual(
    resolveNativeDeepLink({
      decision: "DENY",
      reason_code: "DEEPLINK_AUTHORIZATION_DENY",
      canonical_web_fallback: "https://apgic.ru/login",
      expires_at: "2026-09-24T13:00:00Z",
    }),
    {
      action: "OPEN_WEB_FALLBACK",
      target: "https://apgic.ru/login",
    },
  );
});

test("workspace is visible only after explicit server authorization", () => {
  assert.equal(
    consumeAuthorizedWorkspace({
      allowed: false,
      reason_code: "WORKSPACE_AUTHORIZATION_DENY",
    }),
    null,
  );

  const workspace = {
    workspace_id: "workspace-1",
    identity_id: "identity-1",
    tenant_id: "tenant-1",
    kind: "SPECIALIST" as const,
    authorization_decision: "ALLOW" as const,
    reason_code: "WORKSPACE_ALLOWED",
  };
  assert.deepEqual(
    consumeAuthorizedWorkspace({
      allowed: true,
      reason_code: "WORKSPACE_ALLOWED",
      workspace,
    }),
    workspace,
  );
});

test("native analytics keeps business semantics outside platform diagnostics", () => {
  const event = withNativeAnalyticsDiagnostics(
    {
      event_name: "workspace_switched",
      event_version: "1",
      occurred_at: "2026-09-24T12:00:00Z",
      journey_id: "journey-1",
      identity_ref: "identity-1",
      business_properties: { workspace_kind: "SPECIALIST" },
    },
    "ANDROID",
    { app_version: "1.0.0", network: "wifi" },
  );

  assert.equal(event.event_name, "workspace_switched");
  assert.deepEqual(event.business_properties, { workspace_kind: "SPECIALIST" });
  assert.deepEqual(event.platform_extensions, {
    ANDROID: { app_version: "1.0.0", network: "wifi" },
  });
});
