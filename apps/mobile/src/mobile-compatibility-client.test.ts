import assert from "node:assert/strict";
import test from "node:test";

import {
  fetchClientCompatibility,
  isCompatibilityDecision,
} from "./mobile-compatibility-client.ts";

test("accepts supported previous contract without forced-update fields", () => {
  assert.equal(
    isCompatibilityDecision({
      status: "DEPRECATED_BUT_SUPPORTED",
      reason_code: "CLIENT_VERSION_DEPRECATED",
      policy_version: "mobile-compat-v7",
      contract_version: "contract-v2",
    }),
    true,
  );
});

test("requires governed reason and https URL for forced update", () => {
  assert.equal(
    isCompatibilityDecision({
      status: "UPDATE_REQUIRED",
      reason_code: "CLIENT_CONTRACT_UNSUPPORTED",
      policy_version: "mobile-compat-v7",
      contract_version: "contract-v2",
      update_reason: "INCOMPATIBLE_CRITICAL",
      update_url: "https://apgic.ru/update",
    }),
    true,
  );
  assert.equal(
    isCompatibilityDecision({
      status: "UPDATE_REQUIRED",
      reason_code: "CLIENT_CONTRACT_UNSUPPORTED",
      policy_version: "mobile-compat-v7",
      contract_version: "contract-v2",
      update_reason: "MARKETING",
      update_url: "http://example.test/update",
    }),
    false,
  );
});

test("fetches canonical compatibility decision with encoded client identity", async () => {
  const originalFetch = globalThis.fetch;
  let requestedURL = "";
  globalThis.fetch = (async (input: string | URL | Request) => {
    requestedURL = String(input);
    return new Response(
      JSON.stringify({
        status: "SUPPORTED",
        reason_code: "CLIENT_VERSION_SUPPORTED",
        policy_version: "mobile-compat-v7",
        contract_version: "contract-v2",
      }),
      {status: 200, headers: {"content-type": "application/json"}},
    );
  }) as typeof fetch;

  try {
    const decision = await fetchClientCompatibility({
      baseURL: "https://apgic.ru/",
      platform: "IOS",
      appVersion: "1.6.0",
      buildNumber: "1",
      contractVersion: "contract-v1",
    });
    assert.equal(decision.status, "SUPPORTED");
    assert.match(requestedURL, /platform=IOS/);
    assert.match(requestedURL, /app_version=1.6.0/);
    assert.match(requestedURL, /build_number=1/);
    assert.match(requestedURL, /contract_version=contract-v1/);
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("fails closed on unavailable policy instead of assuming compatibility", async () => {
  const originalFetch = globalThis.fetch;
  globalThis.fetch = (async () =>
    new Response(
      JSON.stringify({code: "CLIENT_COMPATIBILITY_POLICY_UNAVAILABLE"}),
      {status: 503, headers: {"content-type": "application/json"}},
    )) as typeof fetch;

  try {
    await assert.rejects(
      fetchClientCompatibility({
        baseURL: "https://apgic.ru",
        platform: "ANDROID",
        appVersion: "1.6.0",
        contractVersion: "contract-v1",
      }),
      /CLIENT_COMPATIBILITY_HTTP_503/,
    );
  } finally {
    globalThis.fetch = originalFetch;
  }
});
