import assert from "node:assert/strict";
import test from "node:test";

import {
  assertHelpIntentSafety,
  createMobileHelpIntent,
  runMobileDemandE2E,
} from "./mobile-demand-client.ts";

function response(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: {"content-type": "application/json"},
  });
}

test("native HelpIntent client rejects diagnosis assertion", () => {
  assert.throws(
    () =>
      assertHelpIntentSafety({
        id: "intent-1",
        client_identity_id: "identity-1",
        free_text: "Мне тревожно",
        topics: ["anxiety"],
        goals: [],
        context: {},
        status: "DRAFT",
        reason_codes: [],
        notice: "Это предположение, не диагноз.",
        diagnosis_asserted: true,
        catalog_mode: "CONFORMANCE",
      }),
    /HELP_INTENT_DIAGNOSIS_ASSERTED/,
  );
});

test("native HelpIntent E2E preserves same Identity and explicit topic correction", async () => {
  const originalFetch = globalThis.fetch;
  let call = 0;
  globalThis.fetch = async (_input, init) => {
    call += 1;
    if (call === 1) {
      const body = JSON.parse(String(init?.body || "{}")) as {free_text?: string};
      assert.equal(body.free_text, "Мне тревожно и плохо сплю");
      return response(
        {
          id: "intent-1",
          client_identity_id: "identity-1",
          free_text: body.free_text,
          topics: ["anxiety"],
          goals: ["reduce_anxiety"],
          context: {format: "ONLINE", language: "RU"},
          status: "DRAFT",
          reason_codes: ["INTERPRETATION_LEXICON", "NOT_A_DIAGNOSIS"],
          notice: "Это предположение по вашим словам, не диагноз.",
          diagnosis_asserted: false,
          catalog_mode: "CONFORMANCE",
        },
        201,
      );
    }
    const body = JSON.parse(String(init?.body || "{}")) as {topics?: string[]};
    assert.deepEqual(body.topics, ["sleep"]);
    return response({
      id: "intent-1",
      client_identity_id: "identity-1",
      free_text: "Мне тревожно и плохо сплю",
      topics: ["sleep"],
      goals: ["reduce_anxiety"],
      context: {format: "ONLINE", language: "RU"},
      status: "CONFIRMED",
      reason_codes: ["INTERPRETATION_LEXICON", "NOT_A_DIAGNOSIS", "HELP_INTENT_CONFIRMED"],
      notice: "Это предположение по вашим словам, не диагноз.",
      diagnosis_asserted: false,
      catalog_mode: "CONFORMANCE",
    });
  };

  try {
    const result = await runMobileDemandE2E({
      baseURL: "https://example.invalid",
      sessionCookie: "__Host-apgic_session=test",
      freeText: "Мне тревожно и плохо сплю",
      correctedTopics: ["sleep"],
    });
    assert.equal(call, 2);
    assert.equal(result.identityID, "identity-1");
    assert.equal(result.intentID, "intent-1");
    assert.deepEqual(result.originalTopics, ["anxiety"]);
    assert.deepEqual(result.confirmedTopics, ["sleep"]);
    assert.equal(result.diagnosisAsserted, false);
    assert.equal(result.userCorrectionObserved, true);
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("native HelpIntent create surfaces safe backend errors", async () => {
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async () =>
    response(
      {
        code: "HELP_INTENT_INVALID",
        message_safe: "Запрос не удалось прочитать.",
        correlation_id: "corr-1",
        retryable: false,
      },
      400,
    );
  try {
    await assert.rejects(
      createMobileHelpIntent("", {baseURL: "https://example.invalid"}),
      /Запрос не удалось прочитать/,
    );
  } finally {
    globalThis.fetch = originalFetch;
  }
});
