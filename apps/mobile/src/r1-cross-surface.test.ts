import assert from "node:assert/strict";
import test from "node:test";

import {
  buildNativeDeleteAccountInitiation,
  consumeAuthorizedWorkspace,
  resolveNativeDeepLink,
  runNativeDeletionE2E,
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
    { action: "BLOCK" },
  );
});

test("native deep link blocks ALLOW without canonical destination", () => {
  assert.deepEqual(
    resolveNativeDeepLink({
      decision: "ALLOW",
      reason_code: "DEEPLINK_ALLOWED",
    }),
    { action: "BLOCK" },
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


test("native deletion E2E reaches canonical retained-with-reason state and replay is idempotent", async () => {
  const originalFetch = globalThis.fetch;
  let calls = 0;
  globalThis.fetch = async (_input, init) => {
    calls += 1;
    const body = JSON.parse(String(init?.body || "{}")) as {identity_id?: string; source?: string};
    assert.equal(body.identity_id, "identity-1");
    assert.equal(body.source, "IOS");
    return new Response(JSON.stringify({
      id: "deletion-1",
      identity_id: "identity-1",
      source: "IOS",
      state: "PARTIALLY_RETAINED_WITH_REASON",
      deactivation: false,
      profile_erased: true,
      ledger_retained: true,
      provider_evidence: "provider-erasure:identity-1",
      apgic_deletes_ledger: false,
      idempotent: calls > 1,
    }), {status: calls === 1 ? 201 : 200, headers: {"content-type": "application/json"}});
  };

  try {
    const result = await runNativeDeletionE2E({
      baseURL: "https://example.invalid",
      sessionCookie: "__Host-apgic_session=test",
      requestID: "request-1",
      identityID: "identity-1",
      platform: "IOS",
    });
    assert.equal(calls, 2);
    assert.deepEqual(result, {
      requestID: "request-1",
      identityID: "identity-1",
      state: "PARTIALLY_RETAINED_WITH_REASON",
      profileErased: true,
      ledgerRetained: true,
      deactivation: false,
      providerEvidence: "provider-erasure:identity-1",
      replayIdempotent: true,
    });
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("native deletion E2E rejects deactivation masquerading as deletion", async () => {
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async () => new Response(JSON.stringify({
    id: "deletion-1",
    identity_id: "identity-1",
    source: "ANDROID",
    state: "PARTIALLY_RETAINED_WITH_REASON",
    deactivation: true,
    profile_erased: true,
    ledger_retained: true,
    provider_evidence: "provider-erasure:identity-1",
    apgic_deletes_ledger: false,
    idempotent: false,
  }), {status: 201, headers: {"content-type": "application/json"}});

  try {
    await assert.rejects(
      runNativeDeletionE2E({
        baseURL: "https://example.invalid",
        sessionCookie: "__Host-apgic_session=test",
        requestID: "request-2",
        identityID: "identity-1",
        platform: "ANDROID",
      }),
      /NATIVE_DELETION_CANONICAL_STATE_INVALID/,
    );
  } finally {
    globalThis.fetch = originalFetch;
  }
});
